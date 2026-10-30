# Local file upload API

Authenticated HTTP file uploads saved into a configured host directory.

## Start

```sh
cp .env.example .env
openssl rand -hex 32
# Put the generated value in API_TOKEN in .env, then verify HOST_UPLOAD_DIR.
docker compose up -d --build
```

The requested default destination is `/dockervol/1-containers/ha/www/ss2`. Paths are case-sensitive, and that directory does not currently exist on this machine. The existing volume root is `/dockervol/1-Containers`, so verify whether `HOST_UPLOAD_DIR` needs an uppercase `C` before starting.

The container runs as UID/GID `1000:1000`; the destination directory must be writable by that user.

## Upload

```sh
curl --fail \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -F "file=@/path/to/file.jpg" \
  http://127.0.0.1:8080/upload
```

Existing files are protected by default. To intentionally replace one:

```sh
curl --fail \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -F "file=@/path/to/file.jpg" \
  'http://127.0.0.1:8080/upload?overwrite=true'
```

Health check:

```sh
curl http://127.0.0.1:8080/health
```

The default maximum request size is 100 MB. Change `MAX_UPLOAD_MB` in `.env` if needed. The service binds only to localhost by default; put it behind HTTPS before exposing it beyond the machine.

## Goodreads book request → Listenarr

Set `LISTENARR_URL` to the address reachable from this container (for example `http://10.0.0.2:4545`) and `LISTENARR_API_KEY` to the Listenarr API key in your local `.env` file. Never commit either token.

```sh
curl -X POST http://127.0.0.1:8069/goodreads/request \
  -H "Authorization: Bearer YOUR_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"url":"https://www.goodreads.com/book/show/BOOK_ID"}'
```

The handler accepts only HTTPS Goodreads **book** URLs. It attempts to read public page JSON-LD metadata, searches Listenarr's audiobook metadata, and **only automatically adds a unique exact title-and-author match with an ASIN**. It checks Listenarr for duplicates and submits `monitored: true`, `autoSearch: true`. A successful response means Listenarr accepted the add/search request, not that a download is finished.

If Goodreads displays a login page or blocks server fetches, this endpoint returns an error without adding anything. Ambiguous matches return HTTP 409 with candidate details; they are not auto-downloaded. The endpoint uses your existing `API_TOKEN` bearer authentication. It does not require your Goodreads login credentials.

Rebuild and restart the API after pulling updates: `docker compose up -d --build`. For a Swarm image-based deployment, rebuild/push your image and redeploy the service instead.

