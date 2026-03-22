# paste skill

Use this skill to create pastes on the paste service. It handles both text snippets and image uploads.

## Configuration (environment variables)

| Variable | Description | Required |
|----------|-------------|----------|
| `PASTE_URL` | Base URL of the paste service (e.g. `https://paste.example.com`) | Yes |
| `PASTE_USER_ID` | Your user ID for API authentication | Yes |
| `PASTE_API_KEY` | Your API key for authentication | Yes |

## Authentication

All API requests require two headers:
- `X-PASTE-USERID: <PASTE_USER_ID>`
- `X-PASTE-API-KEY: <PASTE_API_KEY>`

## Defaults

Unless the user specifies otherwise:
- `visibility`: `logged_in`
- `expiration`: `1day`

Available visibility values: `public`, `private`, `logged_in`
Available expiration values: `never`, `1hour`, `1day`, `1week`, `1month`, `6months`, `1year`

## Creating a text paste

`POST $PASTE_URL/api/create`
Content-Type: application/json

```json
{
  "content": "your text here",
  "visibility": "logged_in",
  "expiration": "1week",
  "title": "optional title",
  "customSlug": "optional-slug"
}
```

### Example (bash)

```bash
curl -s -X POST "$PASTE_URL/api/create" \
  -H "Content-Type: application/json" \
  -H "X-PASTE-USERID: $PASTE_USER_ID" \
  -H "X-PASTE-API-KEY: $PASTE_API_KEY" \
  -d '{
    "content": "hello world",
    "visibility": "logged_in",
    "expiration": "1week"
  }'
```

## Uploading an image paste

`POST $PASTE_URL/api/upload`
Content-Type: multipart/form-data

Form fields:
- `file` (required): the image file. Supported types: `image/jpeg`, `image/png`, `image/gif`, `image/webp`, `image/svg+xml`. Max size: 10 MB.
- `visibility` (optional, default `logged_in`)
- `expiration` (optional, default `1day`)
- `title` (optional)
- `customSlug` (optional)

### Example (bash)

```bash
curl -s -X POST "$PASTE_URL/api/upload" \
  -H "X-PASTE-USERID: $PASTE_USER_ID" \
  -H "X-PASTE-API-KEY: $PASTE_API_KEY" \
  -F "file=@/path/to/image.png" \
  -F "visibility=logged_in" \
  -F "expiration=1week"
```

## Response format

Both endpoints return JSON:

```json
{
  "success": true,
  "slug": "abc12",
  "url": "/p/abc12"
}
```

On error:
```json
{
  "error": "description of the error"
}
```

## Getting the full URL

The `url` field in the response is a relative path. Construct the full URL by prepending `$PASTE_URL`:

```
FULL_URL="${PASTE_URL}$(echo "$RESPONSE" | jq -r '.url')"
```

**Always output the full URL** so the user can navigate directly to the paste.

## Complete example (text paste)

```bash
RESPONSE=$(curl -s -X POST "$PASTE_URL/api/create" \
  -H "Content-Type: application/json" \
  -H "X-PASTE-USERID: $PASTE_USER_ID" \
  -H "X-PASTE-API-KEY: $PASTE_API_KEY" \
  -d "{\"content\": \"$(cat file.txt | jq -Rs .)\", \"visibility\": \"logged_in\", \"expiration\": \"1day\"}")

SLUG=$(echo "$RESPONSE" | jq -r '.slug')
FULL_URL="${PASTE_URL}/p/${SLUG}"
echo "Paste created: $FULL_URL"
```

## Complete example (image upload)

```bash
RESPONSE=$(curl -s -X POST "$PASTE_URL/api/upload" \
  -H "X-PASTE-USERID: $PASTE_USER_ID" \
  -H "X-PASTE-API-KEY: $PASTE_API_KEY" \
  -F "file=@screenshot.png" \
  -F "visibility=logged_in" \
  -F "expiration=1week")

SLUG=$(echo "$RESPONSE" | jq -r '.slug')
FULL_URL="${PASTE_URL}/p/${SLUG}"
echo "Image uploaded: $FULL_URL"
```