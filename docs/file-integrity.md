# File integrity (`discloud-sha256-v1`)

New uploads store a whole-file digest in `files.sha256` (JSON field `sha256`). The algorithm is **`discloud-sha256-v1`**, implemented in [`internal/integrity/digest.go`](../internal/integrity/digest.go).

This is **not** always the same as `sha256sum` of the raw file.

## Construction

1. Split the file into chunks of the upload’s **stored chunk size** (last chunk may be shorter). Legacy default is **8 MiB** when no guild was configured.
2. Let `H_i` be the SHA-256 of chunk `i` bytes (hex), and `S_i` its length in bytes.
3. If there is **one** chunk: digest = `H_0` (same as `sha256sum` of the file).
4. If there are **two or more** chunks: digest = SHA-256 of the concatenation  

   `BE64(S_0) || raw32(H_0) || BE64(S_1) || raw32(H_1) || …`

   where `BE64` is a big-endian `uint64` size, and `raw32` is the 32-byte decode of the hex chunk hash.

## Verification

```bash
discloud files verify ./downloaded.bin "$(curl -sS "$API/api/files/$ID" | jq -r .sha256)"
```

Or compare against a local digest computed with the same algorithm (`FileSHA256FromReader(r, chunkSize)` / CLI verify). Using plain `sha256sum` is only correct for **single-chunk** files (size ≤ that file’s chunk size). Multi-chunk verify must use the file’s stored `chunkSize`.

## API behavior

- Complete bodies may send `fileSha256` as a hint; mismatch → **400**.
- Legacy rows may omit `sha256` (null until re-uploaded).
- Downloads set `X-Content-SHA256` and `Digest: sha-256=…` (base64 of the raw digest) when the digest is known.

## Self-managed `.discloud` bundles

Self-managed uploads store a whole-file SHA-256 of the raw file bytes (same as `sha256sum`) plus per-chunk SHA-256 in the bundle metadata. Bundle containers also carry integrity tags (HMAC for obfuscated mode; Poly1305 for password mode).

```bash
discloud files verify ./backup.discloud
discloud files verify ./restored.bin ./backup.discloud --file restored.bin
```
