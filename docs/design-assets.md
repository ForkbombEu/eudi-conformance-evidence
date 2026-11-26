# Credimi design assets

The canonical Credimi Extras assets are vendored from [`ForkbombEu/credimi-extras-template`](https://github.com/ForkbombEu/credimi-extras-template) in `brand/`. The embedded runtime copies live in `cmd/webui/static/`; the browser loads `style.css` before `app.css`.

| Canonical asset | Runtime copy | SHA-256 |
| --- | --- | --- |
| `brand/style.css` | `cmd/webui/static/style.css` | `d8cd89b082f4bb82c3e8c8875769abb524cc605ed4b33af05008ddc5c1f941a6` |
| `brand/logos/credimi_logo.svg` | `cmd/webui/static/credimi_logo.svg` | `031885760a9165e9d8d49eab45baca30ba5ed8dd1fbf0b4699fba2de5dc4feac` |
| `brand/logos/credimi_logo_negative.svg` | `cmd/webui/static/credimi_logo_negative.svg` | `32df33f9f5ffa696d452e1f65f5d6738b920415c5114db4b010af1f997a8cb3a` |
| `brand/logos/credimi_logo-transp.svg` | `cmd/webui/static/credimi_logo-transp.svg` | `8407a3ed0beddc137599f71498f1ca8e68766a2684dba80093ea25e17173eef7` |
| `brand/logos/credimi_logo-transp_white.svg` | `cmd/webui/static/credimi_logo-transp_white.svg` | `196017744fca7d3836720995aca8c55c8531908e8dcdafc5ca5e7b48ef8f7e10` |
| `brand/fonts/InterVariable.ttf` | `cmd/webui/static/fonts/InterVariable.ttf` | `29160a80ff49ddcab2c97711247e08b1fab27a484a329ce8b813d820dc559031` |
| `brand/fonts/SourceCodeProVariable.ttf` | `cmd/webui/static/fonts/SourceCodeProVariable.ttf` | `b400fc584e10aff25d0e775ce181b4fc1c5ea1b5dc37b81aeb2084375b945790` |

The regular mark is used on light surfaces and as the same-origin favicon. The negative mark is reserved for the indigo footer. The transparent wordmark identifies the topbar; its white variant closes the footer's Credimi Extras strip. Update the canonical `brand/` asset and synchronize its runtime copy intentionally when the shared design changes.
