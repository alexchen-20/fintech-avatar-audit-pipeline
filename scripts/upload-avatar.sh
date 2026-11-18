#!/bin/sh
set -eu

: "${AVATAR_FILE:?set AVATAR_FILE to an image path}"

curl --fail-with-body --request POST http://localhost:8080/avatars \
  --header "Idempotency-Key: avatar-demo-001" \
  --form "user_id=usr_demo_42" \
  --form "payment_event_id=evt_demo_17" \
  --form "payment_event_type=payment.reversed" \
  --form "risk_action=account-lock" \
  --form "file=@${AVATAR_FILE}"
