package main

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "html"
 "io"
 "net/http"
 "net/url"
 "regexp"
 "strconv"
 "strings"
 "time"
)

var goodreadsBookPath = regexp.MustCompile(`^/book/show/([0-9]+)(?:[.-].*)?/?$`)
var ldJSON = regexp.MustCompile(`(?is)<script[^>]+type=["']application/ld\+json["'][^>]*>(.*?)</script>`)

type bookRequest struct { URL string `json:"url"` }
type bookDetails struct { Title string; Author string; ID string }
type metadataBook struct {
 Title string `json:"title"`
 Author string `json:"author"`
 Asin string `json:"asin"`
}
type matchCandidate struct {
 Title string `json:"title"`
 Author string `json:"author"`
 Asin string `json:"asin"`
}

func parseGoodreadsURL(raw string) (string, string, error) {
 u,err:=url.Parse(strings.TrimSpace(raw))
 if err!=nil || u.Scheme!="https" || (u.Hostname()!="www.goodreads.com" && u.Hostname()!="goodreads.com") || u.User!=nil || u.Port()!="" {
  return "","",errors.New("a https://www.goodreads.com/book/show/... URL is required")
 }
 m:=goodreadsBookPath.FindStringSubmatch(u.Path)
 if m==nil {return "","",errors.New("URL must point to a Goodreads book, not a shelf or profile")}
 return "https://www.goodreads.com/book/show/"+m[1],m[1],nil
}

func getGoodreadsBook(ctx context.Context, client *http.Client, raw string) (bookDetails,error) {
 canonical,id,err:=parseGoodreadsURL(raw)
 if err!=nil{return bookDetails{},err}
 req,err:=http.NewRequestWithContext(ctx,http.MethodGet,canonical,nil)
 if err!=nil{return bookDetails{},err}
 req.Header.Set("User-Agent","Mozilla/5.0 (compatible; personal-book-request/1.0)")
 res,err:=client.Do(req)
 if err!=nil{return bookDetails{},fmt.Errorf("Goodreads unavailable: %w",err)}
 defer res.Body.Close()
 if res.StatusCode!=200 {return bookDetails{},fmt.Errorf("Goodreads returned HTTP %d; the page may require login",res.StatusCode)}
 data,err:=io.ReadAll(io.LimitReader(res.Body,3*1024*1024+1))
 if err!=nil{return bookDetails{},err}
 if len(data)>3*1024*1024{return bookDetails{},errors.New("Goodreads page too large")}
 page:=string(data)
 for _,m:=range ldJSON.FindAllStringSubmatch(page,-1) {
  var item struct {
   Type any `json:"@type"`
   Name string `json:"name"`
   Author json.RawMessage `json:"author"`
  }
  if json.Unmarshal([]byte(html.UnescapeString(m[1])),&item)!=nil || item.Name=="" {continue}
  var authors []struct{Name string `json:"name"`}
  var one struct{Name string `json:"name"`}
  if json.Unmarshal(item.Author,&authors)==nil && len(authors)>0 && authors[0].Name!="" {
   return bookDetails{Title:html.UnescapeString(item.Name),Author:authors[0].Name,ID:id},nil
  }
  if json.Unmarshal(item.Author,&one)==nil && one.Name!="" {
   return bookDetails{Title:html.UnescapeString(item.Name),Author:one.Name,ID:id},nil
  }
 }
 return bookDetails{},errors.New("could not read title and author from Goodreads (login or changed page format); nothing was added")
}

func normalizeBook(s string) string {
 s=strings.ToLower(s)
 var b strings.Builder
 lastSpace:=true
 for _,r:=range s {
  if r>='a'&&r<='z' || r>='0'&&r<='9' {b.WriteRune(r);lastSpace=false} else if !lastSpace {b.WriteByte(' ');lastSpace=true}
 }
 return strings.TrimSpace(b.String())
}

func (s *server) goodreadsRequest(w http.ResponseWriter,r *http.Request) {
 if s.listenarrURL=="" || s.listenarrKey=="" {writeError(w,503,"LISTENARR_URL and LISTENARR_API_KEY must be configured");return}
 r.Body=http.MaxBytesReader(w,r.Body,4096)
 var input bookRequest
 decoder:=json.NewDecoder(r.Body)
 decoder.DisallowUnknownFields()
 if err:=decoder.Decode(&input);err!=nil {writeError(w,400,"expected JSON object with a Goodreads book URL");return}
 if _,_,err:=parseGoodreadsURL(input.URL);err!=nil {writeError(w,400,err.Error());return}
 ctx,cancel:=context.WithTimeout(r.Context(),25*time.Second)
 defer cancel()
 client:=&http.Client{Timeout:12*time.Second,CheckRedirect:func(req *http.Request,via []*http.Request) error {
  if len(via)>=4 || !(req.URL.Hostname()=="goodreads.com"||req.URL.Hostname()=="www.goodreads.com") || req.URL.Scheme!="https" {return errors.New("unexpected Goodreads redirect")}
  return nil
 }}
 book,err:=getGoodreadsBook(ctx,client,input.URL)
 if err!=nil{writeError(w,502,err.Error());return}
 query:=url.Values{}
 query.Set("query",book.Title+" "+book.Author)
 query.Set("returnLimit","10")
 query.Set("fuzzyThreshold","0.7")
 var results []matchCandidate
 if err:=s.listenarrJSON(ctx,http.MethodGet,"/api/v1/search/intelligent?"+query.Encode(),nil,&results);err!=nil {
  writeError(w,502,"Listenarr metadata search failed: "+err.Error());return
 }
 var matches []matchCandidate
 for _,candidate:=range results {
  if candidate.Asin!="" && normalizeBook(candidate.Title)==normalizeBook(book.Title) && normalizeBook(candidate.Author)==normalizeBook(book.Author) {
   matches=append(matches,candidate)
  }
 }
 if len(matches)!=1 {
  writeJSON(w,409,map[string]any{"error":"no unique exact title/author audiobook match; nothing was added","goodreads":book,"candidates":results})
  return
 }
 selected:=matches[0]
 // Avoid duplicate additions using Listenarr's ASIN lookup.
 var existing json.RawMessage
 checkErr:=s.listenarrJSON(ctx,http.MethodGet,"/api/v1/library/by-asin/"+url.PathEscape(selected.Asin),nil,&existing)
 if checkErr==nil && len(existing)>0 && string(existing)!="null" {
  writeJSON(w,200,map[string]any{"status":"already_exists","title":selected.Title,"asin":selected.Asin})
  return
 }
 var remoteErr *listenarrHTTPError
 if checkErr!=nil && (!errors.As(checkErr,&remoteErr)||remoteErr.Status!=404) {
  writeError(w,502,"Listenarr duplicate check failed: "+checkErr.Error());return
 }
 payload:=map[string]any{"metadata":map[string]any{"title":selected.Title,"author":selected.Author,"asin":selected.Asin},"monitored":true,"autoSearch":true}
 var response json.RawMessage
 if err:=s.listenarrJSON(ctx,http.MethodPost,"/api/v1/library/add",payload,&response);err!=nil{
  writeError(w,502,"Listenarr rejected the add request: "+err.Error());return
 }
 writeJSON(w,201,map[string]any{"status":"added_search_requested","goodreadsId":book.ID,"title":selected.Title,"author":selected.Author,"asin":selected.Asin,"listenarr":response})
}

type listenarrHTTPError struct {Status int}
func (e *listenarrHTTPError) Error() string {return "HTTP "+strconv.Itoa(e.Status)}
func (s *server) listenarrJSON(ctx context.Context,method,path string,body any,out any) error {
 var payload io.Reader
 if body!=nil {raw,err:=json.Marshal(body);if err!=nil{return err};payload=bytes.NewReader(raw)}
 req,err:=http.NewRequestWithContext(ctx,method,strings.TrimRight(s.listenarrURL,"/")+path,payload)
 if err!=nil{return err}
 req.Header.Set("X-Api-Key",s.listenarrKey)
 if body!=nil {req.Header.Set("Content-Type","application/json")}
 client:=&http.Client{Timeout:15*time.Second}
 res,err:=client.Do(req)
 if err!=nil{return err}
 defer res.Body.Close()
 if res.StatusCode<200 || res.StatusCode>=300 {return &listenarrHTTPError{Status:res.StatusCode}}
 raw,err:=io.ReadAll(io.LimitReader(res.Body,4*1024*1024))
 if err!=nil{return err}
 if len(bytes.TrimSpace(raw))==0{return nil}
 if out!=nil{return json.Unmarshal(raw,out)}
 return nil
}
