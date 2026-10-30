package main

import (
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"
)

func TestParseGoodreadsURL(t *testing.T) {
 tests:=[]struct{input,id string; ok bool}{
  {"https://www.goodreads.com/book/show/12345-the-book","12345",true},
  {"https://goodreads.com/book/show/12345?ref=abc","12345",true},
  {"http://goodreads.com/book/show/12345","",false},
  {"https://goodreads.com/review/list/52505829","",false},
  {"https://goodreads.com.evil.invalid/book/show/12345","",false},
  {"https://goodreads.com:444/book/show/12345","",false},
 }
 for _,tc:=range tests {
  _,id,err:=parseGoodreadsURL(tc.input)
  if (err==nil)!=tc.ok || id!=tc.id {t.Errorf("%q = id %q err %v",tc.input,id,err)}
 }
}

func TestGoodreadsEndpointRequiresToken(t *testing.T) {
 s:=&server{token:"secret"}
 req:=httptest.NewRequest(http.MethodPost,"/goodreads/request",strings.NewReader(`{"url":"https://www.goodreads.com/book/show/123"}`))
 rec:=httptest.NewRecorder()
 s.authorize(s.goodreadsRequest)(rec,req)
 if rec.Code!=401 {t.Fatalf("got %d, want 401",rec.Code)}
}

func TestGoodreadsEndpointNeedsConfig(t *testing.T) {
 s:=&server{}
 req:=httptest.NewRequest(http.MethodPost,"/goodreads/request",strings.NewReader(`{"url":"https://www.goodreads.com/book/show/123"}`))
 rec:=httptest.NewRecorder()
 s.goodreadsRequest(rec,req)
 if rec.Code!=503 {t.Fatalf("got %d, want 503",rec.Code)}
}
