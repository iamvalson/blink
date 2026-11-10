# Instagram publishing setup

Blink uses Instagram Login with the `instagram_business_basic` and
`instagram_business_content_publish` permissions. Use an Instagram professional
account assigned to the Meta app for the first end-to-end test. Request the
appropriate Advanced Access through Meta App Review before enabling customer
accounts. Use the current [Instagram Login documentation](https://developers.facebook.com/docs/instagram-platform/instagram-api-with-instagram-login/)
to confirm eligibility and the pinned Graph API version.

1. Copy the Instagram App ID and Instagram App Secret into the backend
   environment on both the API and worker. See `.env.example` for the variable
   names. Set `INSTAGRAM_API_VERSION` to a supported version and
   `INSTAGRAM_CALLBACK_URL` to
   `https://<public-api-host>/auth/instagram/callback`. Register that exact URL
   in the Instagram Business Login settings.
2. Apply migration 16 with `make migrate-up` before deploying the API and
   worker. Use Redis 6.2 or newer for single-use OAuth state, a private R2 bucket
   for media, and install `ffprobe` on the worker. R2 read URLs are signed at
   publish time. Keep their query strings out of proxy logs.
3. Register the deauthorization URL
   `https://<public-api-host>/webhooks/instagram/deauthorize` and deletion
   request URL `https://<public-api-host>/webhooks/instagram/data-deletion`.
   Confirm the signing secret and callback identity with an actual Meta test
   before going live. Deletion returns a confirmation code and status URL.
   Publish the app's privacy and deletion instructions in Meta's dashboard.
4. If you want to verify an event callback, set
   `INSTAGRAM_WEBHOOK_VERIFY_TOKEN` to a random secret and `META_APP_SECRET`
   to the subscription app's signing secret on the API. After deployment, use
   `https://<public-api-host>/webhooks/instagram` in Meta's Callback URL field.
   Enter the same verify token and leave the client-certificate option off
   unless the proxy is configured for mutual TLS. The publishing integration
   polls container status and does not require subscriptions to event fields.
5. Connect the tester in Blink's Accounts page. Publish a JPEG photo, schedule
   an MP4 Reel, and check the progress and result link. Test a carousel and a
   Story on an eligible account before offering either format to users. Verify
   the assets can be fetched by Meta through the temporary R2 URLs.

A disconnected Instagram account is removed from Blink. To revoke the grant
at Instagram as well, remove Blink from Instagram's connected apps. If Blink
marks a publication as requiring manual review, inspect Instagram before
creating a replacement. A publish timeout may mean Meta accepted the post.

The local test suite includes the webhook challenge, signature verification,
a simulated Graph API, and PostgreSQL persistence for ordered media and recovery.
Set `BLINK_TEST_DATABASE_URL` and `BLINK_TEST_REDIS_URL` to local test services
to exercise the database and queue integration tests. Dashboard approval,
real media retrieval, and publication on an Instagram account require a live
test.
