console.log(
  "%cEUDI Evidence%c protocol context extractor",
  "background:#220E7E;color:#FAFAFA;padding:5px 9px;font-weight:800;border-radius:6px 0 0 6px;",
  "background:#EEEAFE;color:#2F294B;padding:5px 9px;font-weight:700;border-radius:0 6px 6px 0;",
);

for (const button of document.querySelectorAll("[data-copy-target]")) {
  button.addEventListener("click", async (event) => {
    event.preventDefault();
    event.stopPropagation();
    const target = document.getElementById(button.dataset.copyTarget);
    if (!target) return;
    const value = target instanceof HTMLTextAreaElement ? target.value : target.textContent ?? "";
    await navigator.clipboard.writeText(value);
    const label = button.getAttribute("aria-label") ?? "Copy to clipboard";
    button.setAttribute("aria-label", "Copied");
    window.setTimeout(() => { button.setAttribute("aria-label", label); }, 1200);
  });
}

for (const button of document.querySelectorAll("[data-dismiss-alert]")) {
  button.addEventListener("click", () => button.closest(".alert")?.setAttribute("hidden", ""));
}

const tabs = [...document.querySelectorAll('[role="tab"]')];

function activateTab(tab) {
  for (const candidate of tabs) {
    const selected = candidate === tab;
    candidate.setAttribute("aria-selected", String(selected));
    candidate.tabIndex = selected ? 0 : -1;
    document.getElementById(candidate.getAttribute("aria-controls"))?.toggleAttribute("hidden", !selected);
  }
}

for (const [index, tab] of tabs.entries()) {
  tab.addEventListener("click", () => activateTab(tab));
  tab.addEventListener("keydown", (event) => {
    let nextIndex;
    if (event.key === "ArrowRight") nextIndex = (index + 1) % tabs.length;
    if (event.key === "ArrowLeft") nextIndex = (index - 1 + tabs.length) % tabs.length;
    if (event.key === "Home") nextIndex = 0;
    if (event.key === "End") nextIndex = tabs.length - 1;
    if (nextIndex === undefined) return;
    event.preventDefault();
    tabs[nextIndex].focus();
    activateTab(tabs[nextIndex]);
  });
}
