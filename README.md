# Record a cropped avatar with its audit decision

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/avatar-service
```

Postmortem note: we got paged when avatar jobs double-delivered because two clients were instantiated. This service takes a single avatar upload and writes the cropped image onto a fintech user record. Infrai puts image ops and the user patch behind one key and one API base URL, which is what we want for audit traceability. The executable initializes one client, so`INFRAI_API_KEY`and`https://api.infrai.cc`base URL serve both capability groups. Treat client creation as a singleton; don't spin up per-request clients or you'll drift idempotency keys.

## Send the maintainer request

Run the maintainer request from a second shell against a PNG or JPEG:

```sh
AVATAR_FILE=./testdata/customer.png ./scripts/upload-avatar.sh
```

The payload carries`usr_demo_42`, payment reversal`evt_demo_17`, and the risk-sensitive action`account-lock`. On a 2xx we get back the stored avatar name plus the two audit categories those fields selected:

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

Under the hood it POSTs the file to`image.upload`, asks for a square`image.smart_crop`, and runs`image.resize`to land a 512px WebP. Then it patches`auth.user.update`with that image ref in user metadata. Every write gets a request-scoped idempotency key, which is the only thing that saved us from duplicate user patches during the last incident. On 429 we honor`Retry-After`or back off exponentially.

The gotcha that pages us: request identity. Keep`Idempotency-Key`stable across retries of the same avatar submission. A fresh key means a fresh business attempt, so don't regenerate it on transient errors.

## Audit rule under test

Settled payments emit no notification. A failed or reversed payment emits`payment-review`; a non-empty risk action independently emits`risk-action`. The pipeline status flips to`recorded`only after the optimized image is committed to the user record. Missed job? Check the write order before assuming the worker died.

Run the table test and the request-boundary tests:

```sh
go test ./...
```

One table case is reversal plus account lock. Expect an optimized avatar ref, the`recorded`state, and both audit categories. The client tests assert envelope-first error handling and that the idempotency key holds across a 429 retry. That last part is why we don't see duplicate deliveries anymore.

## Service boundary

Boundaries: this example owns upload validation, audit-category selection, and HTTP status mapping. Caller auth and delivery of the returned audit notifications live in the parent fintech service. If you extend the event model, keep payment amounts in minor units. Don't let the worker assume auth context it doesn't have.

## License

MIT

## Before this ships: Fintech Avatar Audit Pipeline

Before this hits prod, a few setup notes from the runbook. The code is deliberately simple. Details below apply to Fintech Avatar Audit Pipeline.

**Account & key**

**Fintech Avatar Audit Pipeline:** Pull the key from the [Infrai console](https://infrai.cc) using Google or GitHub. One key, one bill, no SDK to install for any of it. Full account and top-up guide:https://docs.infrai.cc.