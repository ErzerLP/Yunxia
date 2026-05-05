# OpenList PikPak captcha comparison

Sources (primary):

* https://raw.githubusercontent.com/OpenListTeam/OpenList/v4.2.1/drivers/pikpak/driver.go
* https://raw.githubusercontent.com/OpenListTeam/OpenList/v4.2.1/drivers/pikpak/util.go
* https://raw.githubusercontent.com/OpenListTeam/OpenList/v4.2.1/drivers/pikpak/meta.go

Key observations for this fix:

* OpenList default `platform` is `web`.
* Post-login captcha action is `GetAction(GET, https://api-drive.mypikpak.net/drive/v1/files)`, which strips host/query and yields `GET:/drive/v1/files`.
* Captcha init request is sent through the normal provider request helper, so it carries `User-Agent`, `X-Device-ID`, `X-Captcha-Token`, and post-login `Authorization: Bearer <access_token>`.
* Captcha body contains `action`, previous `captcha_token`, `client_id`, `device_id`, `meta`, and `redirect_uri`; query includes `client_id`.
* OpenList first calls post-login captcha after login/refresh and before the first drive list.
* OpenList updates Android user-agent with `usrno=<user_id>` only after post-login captcha. The post-login captcha request itself uses the pre-login Android UA (`usrno/` empty); later drive requests use the user-bound UA.
* OpenList first drive list query always includes `page_token` (empty for the first page), in addition to `parent_id`, `thumbnail_size`, `with_audit`, `limit`, and `filters`.
* OpenList docs state PikPak root folder id defaults to `root` when omitted: https://oplist.org/guide/drivers/pikpak
