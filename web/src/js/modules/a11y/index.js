// Accessibility fixes for what Pat's scripts and its libraries put on
// the page after it loads: icon-font glyphs, bootstrap-tokenfield's
// fields, and sortable column headings.
//
// - A glyph with a meaning of its own (a status: a title, or a success or
//   danger colour) is named for screen readers; one beside text is hidden.
// - bootstrap-tokenfield types into a field of its own, which takes the
//   name of the field it replaces; its hidden helper fields are hidden
//   from screen readers too.
// - Column headings that sort on a click also sort with Enter or Space.

function fixIcons(root) {
  root.querySelectorAll('.glyphicon:not([aria-hidden]):not([aria-label])').forEach((icon) => {
    const title = icon.getAttribute('title');
    let label = null;

    if (title) {
      label = title;
    } else if (icon.classList.contains('text-success')) {
      label = 'OK';
    } else if (icon.classList.contains('text-danger')) {
      label = 'Not OK';
    }
    if (label) {
      icon.setAttribute('role', 'img');
      icon.setAttribute('aria-label', label);
    } else {
      icon.setAttribute('aria-hidden', 'true');
    }
  });
}

function fixTokenfields(root) {
  root.querySelectorAll('.tokenfield').forEach((tf) => {
    const original = tf.querySelector('input[aria-label]:not(.token-input)');
    const typing = tf.querySelector('input.token-input');

    if (original && typing && !typing.getAttribute('aria-label')) {
      typing.setAttribute('aria-label', original.getAttribute('aria-label'));
    }
    tf.querySelectorAll('input[tabindex="-1"]').forEach((i) => i.setAttribute('aria-hidden', 'true'));
  });
}

function fixSortHeadings(root) {
  root.querySelectorAll('#folder th:not([tabindex])').forEach((th) => {
    if (!th.textContent.trim()) {
      return;
    }
    th.setAttribute('tabindex', '0');
    th.setAttribute('aria-label', `${th.textContent.trim()}, sort`);
    th.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        th.click();
      }
    });
  });
}

function fixAll() {
  fixIcons(document);
  fixTokenfields(document);
  fixSortHeadings(document);
}

export function initA11y() {
  let pending = false;

  fixAll();
  // Batched: one pass per frame however many changes came.
  new MutationObserver(() => {
    if (!pending) {
      pending = true;
      requestAnimationFrame(() => {
        pending = false;
        fixAll();
      });
    }
  }).observe(document.body, { childList: true, subtree: true });
}
