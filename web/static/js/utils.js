// Global utility functions for LoraFog Dashboard
// Handles toast messages and global htmx events

document.addEventListener("htmx:afterRequest", (evt) => {
  if (evt.detail.successful) {
    showToast("✅ Request successful", false);
  } else {
    showToast("⚠️ Request failed", true);
  }
});

function showToast(message, isError) {
  const toast = document.createElement("div");
  toast.textContent = message;
  toast.className = `toast-enter fixed bottom-5 right-5 px-4 py-2 rounded text-white text-sm shadow-lg z-50 ${
    isError ? "bg-red-500" : "bg-green-600"
  }`;
  document.body.appendChild(toast);

  setTimeout(() => {
    toast.classList.add("toast-leave");
    setTimeout(() => toast.remove(), 400);
  }, 2000);
}
