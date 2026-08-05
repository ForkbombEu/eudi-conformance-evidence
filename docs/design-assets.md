# Credimi design assets

The Go embedded runtime copies are `cmd/webui/static/style.css` (SHA-256 `ff452337f866cae1060057a8c417752b5a9767f59b748c9c7cde509c638387c7`), `cmd/webui/static/credimi_logo.svg` (SHA-256 `031885760a9165e9d8d49eab45baca30ba5ed8dd1fbf0b4699fba2de5dc4feac`), `cmd/webui/static/credimi_logo_negative.svg` (SHA-256 `32df33f9f5ffa696d452e1f65f5d6738b920415c5114db4b010af1f997a8cb3a`), `cmd/webui/static/credimi_logo-transp.svg` (SHA-256 `8407a3ed0beddc137599f71498f1ca8e68766a2684dba80093ea25e17173eef7`), and `cmd/webui/static/credimi_logo-transp_white.svg` (SHA-256 `196017744fca7d3836720995aca8c55c8531908e8dcdafc5ca5e7b48ef8f7e10`). They are unchanged copies of the corresponding `HITL/` inputs.

The regular logo is used on light surfaces and as the `/static/credimi_logo.svg` favicon. The negative logo is reserved for the dark footer. Both remain this application's own topbar and footer brand mark and are unrelated to the Credimi Extras banner below. `app.css` loads after the shared foundation. Replace an HITL input intentionally and synchronize its embedded copy when changing shared branding.

## Credimi Extras banner

Every Credimi Extras application carries the same pair of cross-promotional strips: one above the topbar and one closing the footer. Both read "This app is part of **Credimi Extras**. Automate all your EUDI testing with **Credimi**". Only the final word or wordmark is an `<a href="https://credimi.io" target="_blank" rel="noopener">` — the strip itself is never a click target.

The two wordmark assets exist for this banner. The transparent wordmark `credimi_logo-transp.svg` is the light-background variant of the pair and is installed so that the canonical set stays complete; the white wordmark `credimi_logo-transp_white.svg` is the dark-background variant and is the one this application renders, inline at 16px with `alt="Credimi"` inside the bottom strip over the dark footer. The light top strip does not use a wordmark: it carries the 16px `credimi_logo.svg` mark before the sentence, decorative and `aria-hidden`, and spells "Credimi" as underlined text, exactly as in the other Credimi Extras applications.
