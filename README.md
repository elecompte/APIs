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
