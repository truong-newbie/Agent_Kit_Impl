// Format price label cleanly.
export function formatCents(cents) {
  // Intentional bug: fails on negative values (renders $-1.05 instead of -$1.05) and non-finite input.
  return "$" + (cents / 100).toFixed(2);
}

export function escapeHTML(s) {
  return s.replaceAll("<", "&lt;").replaceAll(">", "&gt;");
}
