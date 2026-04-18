---
title: Alert recipes
description: Ready-made alert configurations for Slack, Discord, and generic JSON sinks.
---

Three recipes for connecting SimpleDeploy alerts to common destinations.

## Slack incoming webhook

Create an [Incoming Webhook](https://api.slack.com/messaging/webhooks) in your Slack workspace. Copy the URL.

In SimpleDeploy: **Alerts > Webhooks > New**. Paste the Slack URL. Save.

Then **Alerts > Rules > New**: e.g., CPU `>` 80 for 300 seconds, scope `all apps`, webhook = the one above.

The Slack webhook accepts SimpleDeploy's default JSON shape. The `message` field becomes the post body. For richer formatting, point your webhook at a small relay that translates SimpleDeploy's payload into Slack Block Kit.

Example translated message:

> :warning: *e2e-postgres* CPU 87.4% (threshold 80%) for 5 min

## Discord webhook

Create a [Discord webhook](https://support.discord.com/hc/en-us/articles/228383668-Intro-to-Webhooks) on the target channel. Copy the URL.

Same setup as Slack: paste the URL, attach to a rule.

Discord ignores fields it does not understand; the default JSON renders as a raw post. For embeds, run a translator that produces:

```json
{
  "embeds": [{
    "title": "CPU above threshold",
    "description": "e2e-postgres",
    "color": 16763904,
    "fields": [
      { "name": "Value", "value": "87.4%" },
      { "name": "Threshold", "value": "80%" },
      { "name": "Duration", "value": "5 min" }
    ]
  }]
}
```

## Generic JSON to a custom endpoint with HMAC

For internal alert routers (PagerDuty, Opsgenie, your own incident bot), accept SimpleDeploy's JSON directly:

```json
{
  "rule_id": 12,
  "metric": "cpu",
  "op": ">",
  "threshold": 80,
  "value": 87.4,
  "app_slug": "e2e-postgres",
  "fired_at": "2026-04-17T14:31:00Z",
  "message": "CPU 87.4% above 80% for 5 min"
}
```

To verify the request came from your SimpleDeploy instance, configure a webhook secret. SimpleDeploy signs the body with HMAC-SHA256 and sends the digest in `X-SimpleDeploy-Signature: sha256=<hex>`. Verify in your handler:

```go
expected := hmac.New(sha256.New, []byte(secret))
expected.Write(body)
got := r.Header.Get("X-SimpleDeploy-Signature")
if !hmac.Equal([]byte("sha256="+hex.EncodeToString(expected.Sum(nil))), []byte(got)) {
    http.Error(w, "bad signature", 401)
    return
}
```

## Tuning

Start broad (CPU > 90 for 5 min). Tune as you learn the noise floor. Avoid alerts that fire more than a few times a day; either raise the threshold or lengthen the duration.
