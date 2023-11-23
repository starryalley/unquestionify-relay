This is a really simple and stupid bitmap cache/relay server aimed to be deployed in the cloud, to be used for [Unquestionify (Garmin Connect IQ watchapp)](https://github.com/starryalley/Unquestionify) and [Unquestionify-Android (Android companion app)](https://github.com/starryalley/Unquestionify-android). 

# Why

Because of [this change](https://forums.garmin.com/developer/connect-iq/i/bug-reports/connect-mobile-4-40-makeimagerequest-localhost-error?CommentSortBy=CreatedDate&CommentSortOrder=Descending) since [GCM](https://www.garmin.com/en-AU/p/125677) (Garmin Connect Mobile app) 4.40, the [`makeImageRequest()`](https://developer.garmin.com/connect-iq/api-docs/Toybox/Communications.html#makeImageRequest-instance_function) API has to go through Garmin's server to do the image processing for the device. This means using URL like http://127.0.0.1/xxx is now impossible in the API, rendering the [Unquestionify app](https://github.com/starryalley/Unquestionify) unsable unless you keep using the ancient GCM version.

This is really a quick workaround to bring Unquestionify back to life without having to use the old GCM, at the cost of making the Companion app uploading the bitmap to this cache/relay, and then the Unquestionify app issuing the `makeImageRequest()` will thus be able to fetch the correct bitmap back to the watch for display.

# How

This is how it works:

- Companion app: receives a notification on the phone. Generating 1-bit png on the fly, upload to this cache server.
- Companion app: notify the watchapp. A prompt will be there asking the user if they want to view the notification.
- Watch app: receives the information about the notification, calling `makeImageRequest()` to download the bitmap back to the watch and display.

## Data flow

Before: Everything lives in the phone. No internet data is used.

- Companion app hosts a http server at 127.0.0.1
- Watchapp (`makeImageRequest()`) -> GCM -> Companion app -> GCM -> Watchapp


Now: you use the phone internet to upload to the cloud, and GCM downloads from the cloud too.

- Companion app -> this cache/relay server (`PUT`/`DELETE` requests)
- Watchapp (`makeImageRequest()`) -> GCM -> Garmin service -> this cache/relay server (`GET` requests) -> GCM -> Watchapp


You see how stupid it is. But this is just a quick workaround without spending too much time.
