console.log(
  "%cEUDI Evidence%c protocol context extractor",
  "background:#312060;color:#fff;padding:5px 9px;font-weight:800;border-radius:6px 0 0 6px;",
  "background:#f1e9f7;color:#22172f;padding:5px 9px;font-weight:700;border-radius:0 6px 6px 0;",
);

for (const button of document.querySelectorAll("[data-copy-target]")) {
  button.addEventListener("click", async () => {
    const target = document.getElementById(button.dataset.copyTarget);
    if (!target) return;
    await navigator.clipboard.writeText(target.textContent ?? "");
    const label = button.textContent;
    button.textContent = "Copied";
    window.setTimeout(() => { button.textContent = label; }, 1200);
  });
}
