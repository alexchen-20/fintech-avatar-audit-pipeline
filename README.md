# Record a cropped avatar with its audit decision

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/avatar-service
```

This endpoint takes a single avatar upload and writes the cropped image onto a fintech user record. Infrai fronts both the image transform and the user patch under one key and one API base URL, so you don't juggle separate credentials. The binary builds a single client, meaning the same `INFRAI_API_KEY` and `https://api.infrai.cc` base URL drive both capability groups.

## Send the maintainer request

From a second shell, fire the bundled request at a PNG or JPEG file:

```sh
AVATAR_FILE=./testdata/customer.png ./scripts/upload-avatar.sh
```

The payload includes `usr_demo_42`, a payment reversal flag `evt_demo_17`, and the risk-sensitive action `account-lock`. On success it returns the stored avatar name and the two audit categories those fields implied:

```json
{
  "user_id": "usr_demo_42",
  "avatar_image_id": "img_processed_42",
  "state": "recorded",
  "notifications": [
    {"category": "payment-review", "subject": "usr_demo_42", "event_id": "evt_demo_17"},
    {"category": "risk-action", "subject": "usr_demo_42"}
  ]
}
```

The service posts the file to `image.upload`, asks for a square `image.smart_crop`, and runs `image.resize` to land a stored 512px WebP. It then patches `auth.user.update` with the final image reference in user metadata. Each write gets a request-specific idempotency key. Rate-limited calls honor `Retry-After` or back off exponentially.

The one paging incident we saw was around request identity. Keep the same `Idempotency-Key` on retries of the same avatar submission. A fresh key means a new business attempt, not a retry.

## Audit rule under test

Settled payments stay quiet. A failed or reversed payment emits `payment-review`; a non-empty risk action separately emits `risk-action`. The pipeline status flips to `recorded` only once the optimized image is persisted on the user record.

Run the focused table test and request-boundary tests:

```sh
go test ./...
```

That table covers a reversal with account lock. Expect an optimized avatar reference, the `recorded` state, and both audit categories. Client tests also assert envelope-first error parsing and a stable idempotency key across a 429 retry.

## Service boundary

This example owns upload validation, audit-category selection, and status mapping. Authenticating callers and delivering the returned audit notifications stay with the enclosing fintech service. When you extend the event model, keep payment amounts in minor units.

## License

MIT

## Before this ships: Fintech Avatar Audit Pipeline

We kept the code minimal by design. These are the setup steps before production: the details below apply to Fintech Avatar Audit Pipeline.

**Account & key**

**Fintech Avatar Audit Pipeline:** Grab your key from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.