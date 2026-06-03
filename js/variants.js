// variants.js — pure helpers for the challenge variant picker.
//
// The server is the single source of truth for which variants exist: it ships
// the registered list (engine.List()) inside the `welcome` message. The frontend
// never hard-codes variant names — it parses them out of that payload here and
// derives display labels, so adding a variant on the backend automatically makes
// it choosable in the UI with no frontend change.
//
// Everything in this module is DOM-free and side-effect-free except
// populateVariantPicker, which is the thin glue that writes the parsed options
// into a <select>. The pure functions are what the node --test suite exercises.

// parseVariants extracts the variant-name list from a `welcome` message,
// tolerating a missing/garbage payload (returns []) and dropping any non-string
// or empty entries. Order is preserved as the server sent it.
export function parseVariants(welcome) {
  if (!welcome || !Array.isArray(welcome.variants)) return [];
  return welcome.variants.filter((v) => typeof v === 'string' && v.length > 0);
}

// variantLabel turns a registry key ("rainbow") into a human-friendly label
// ("Rainbow"). Unknown/empty input yields an empty string.
export function variantLabel(name) {
  if (typeof name !== 'string' || name.length === 0) return '';
  return name.charAt(0).toUpperCase() + name.slice(1);
}

// buildVariantOptions maps a list of variant names to {value, label} pairs ready
// to become <option> elements. It reuses parseVariants' sanitisation so a stray
// non-string entry can never reach the DOM.
export function buildVariantOptions(variants) {
  return parseVariants({ variants }).map((name) => ({
    value: name,
    label: variantLabel(name),
  }));
}

// populateVariantPicker fills a <select> element with the variant options. DOM
// glue — not part of the unit-tested surface; guarded so it no-ops without a
// select or a document (e.g. under the Node test runner).
export function populateVariantPicker(selectEl, variants) {
  if (!selectEl || typeof document === 'undefined') return;
  const previous = selectEl.value;
  selectEl.textContent = '';
  for (const opt of buildVariantOptions(variants)) {
    const el = document.createElement('option');
    el.value = opt.value;
    el.textContent = opt.label;
    selectEl.appendChild(el);
  }
  // Preserve the user's current pick across roster/welcome refreshes if it is
  // still offered; otherwise fall back to the first option.
  if (previous && buildVariantOptions(variants).some((o) => o.value === previous)) {
    selectEl.value = previous;
  }
}
