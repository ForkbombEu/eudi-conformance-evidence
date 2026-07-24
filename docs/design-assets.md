# Credimi design assets

The Go embedded runtime copies are `cmd/webui/static/style.css` (SHA-256 `ff452337f866cae1060057a8c417752b5a9767f59b748c9c7cde509c638387c7`), `cmd/webui/static/credimi_logo.svg` (SHA-256 `031885760a9165e9d8d49eab45baca30ba5ed8dd1fbf0b4699fba2de5dc4feac`), and `cmd/webui/static/credimi_logo_negative.svg` (SHA-256 `32df33f9f5ffa696d452e1f65f5d6738b920415c5114db4b010af1f997a8cb3a`). They are unchanged copies of the corresponding `HITL/` inputs.

The regular logo is used on light surfaces and as the `/static/credimi_logo.svg` favicon. The negative logo is reserved for the dark footer. `app.css` loads after the shared foundation. Replace an HITL input intentionally and synchronize its embedded copy when changing shared branding.
