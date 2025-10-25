// Dashboard enhancement scripts for realtime telemetry
// Adds subtle animations when telemetry updates

document.addEventListener("htmx:afterSwap", (evt) => {
  const target = evt.detail.target;
  if (target && target.id === "telemetry") {
    target.classList.add("ring", "ring-blue-400", "shadow-lg");
    setTimeout(() => {
      target.classList.remove("ring", "ring-blue-400", "shadow-lg");
    }, 600);
  }
});
