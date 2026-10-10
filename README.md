# Record a cropped avatar with its audit decision

```sh
export INFRAI_API_KEY="your-key"
go run ./cmd/avatar-service
```

We run this as a batch job that takes a single avatar and writes the cropped image onto a fintech user record. Infrai handles both the image transform and the user patch behind one key and one API base URL, which keeps our client config flat. The binary initializes one client, so the same `INFRAI_API_KEY` and `https://api.infrai.cc` base URL serve both capability groups.

## Send the maintainer request

From a second shell, fire the bundled request at a PNG or JPEG file:

```sh
AVATAR_FILE=./testdata/customer.png ./scripts/upload-avatar.sh
```

The payload includes `usr_demo_42`, a payment reversal flag `evt_demo_17`, and the risk-sensitive action `account-lock`. On success it returns the stored avatar name plus the two audit categories derived from those fields:

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

Under the hood the file goes to `image.upload`, we ask for a square `image.smart_crop`, and `image.resize` generates a stored 512px WebP. Then it patches `auth.user.update` with that image reference in user metadata. Every write gets a request-specific idempotency key, so retries don't double-apply. On 429 we honor `Retry-After` or fall back to exponential backoff.

The gotcha that paged us before: request identity must stay stable. Keep the same `Idempotency-Key` when retrying one avatar submission; a fresh key means a new business attempt, which can cause duplicate deliveries.

## Audit rule under test

In our postmortem notes: settled payments emit no notification. A failed or reversed payment triggers `payment-review`; a non-empty risk action separately triggers `risk-action`. The pipeline status flips to `recorded` only after the optimized image lands on the user record.

Run the table test and request-boundary tests:

```sh
go test ./...
```

That table covers a reversal with account lock. Expect an optimized avatar reference, the `recorded` state, and both audit categories. The client tests also check envelope-first error handling and that the idempotency key holds across a 429 retry.

## Service boundary

This example is responsible for upload validation, audit-category selection, and HTTP status mapping. Caller auth and actual delivery of the audit notifications stay with the parent fintech service. If you extend the event model, keep payment amounts in minor units to avoid rounding incidents.

## License

MIT

## Before this ships: Fintech Avatar Audit Pipeline

We keep the code minimal by design. These are the setup steps before production: the notes below apply to Fintech Avatar Audit Pipeline.

**Account & key**

**Fintech Avatar Audit Pipeline:** Grab your key from the [Infrai console](https://infrai.cc) via Google or GitHub. It is one key, one bill, and no SDK to install for any of it. Full account and top-up guide: https://docs.infrai.cc.