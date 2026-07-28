// The only bespoke script in the app. Two things the design system genuinely
// cannot do in CSS, and nothing else.
//
// Both are single delegated listeners on the document, so a modal or a menu that
// arrives inside an HTMX fragment works with no re-initialisation — nothing here
// is bound per-element, and nothing needs to run again after a swap.
(function () {
  // --- 1. Native <dialog> needs showModal() to open modally ----------------
  //
  // Everything else the modal needs — Escape to close, focus trapping, making
  // the background inert, returning focus on close — is the browser's own
  // behaviour once showModal() has been used. We do not reimplement any of it.
  document.addEventListener('click', function (e) {
    var opener = e.target.closest('[data-open-dialog]');
    if (opener) {
      var dlg = document.getElementById(opener.getAttribute('data-open-dialog'));
      if (dlg && typeof dlg.showModal === 'function') {
        e.preventDefault();
        // If the opener sits inside an overflow menu, close the menu behind it.
        var owner = opener.closest('details.menu');
        if (owner) owner.open = false;
        dlg.showModal();
      }
      return;
    }

    var closer = e.target.closest('[data-close-dialog]');
    if (closer) {
      var open = closer.closest('dialog');
      if (open) {
        e.preventDefault();
        open.close();
      }
      return;
    }

    // Clicking the backdrop closes: the dialog element itself is the backdrop
    // hit target, since its children cover the panel.
    if (e.target.tagName === 'DIALOG' && typeof e.target.close === 'function') {
      e.target.close();
    }
  });

  // --- 2. The overflow menu is a native <details>, which opens and closes on
  // its own summary but does NOT close on Escape or on a click elsewhere.
  // Both are expected of anything that behaves like a menu, so they are added
  // here rather than left as a papercut. (Design round 4 §1 asks for Escape.)
  var closeMenus = function (except) {
    var menus = document.querySelectorAll('details.menu[open]');
    for (var i = 0; i < menus.length; i++) {
      if (menus[i] !== except) menus[i].open = false;
    }
  };

  document.addEventListener('click', function (e) {
    var inside = e.target.closest('details.menu');
    // A click on a menu item does its own thing and then the menu should shut.
    if (inside && e.target.closest('.menu-item')) {
      closeMenus();
      return;
    }
    closeMenus(inside);
  });

  document.addEventListener('keydown', function (e) {
    if (e.key !== 'Escape') return;
    var open = document.querySelector('details.menu[open]');
    if (!open) return;
    open.open = false;
    // Return focus to the control that opened it, as a menu should.
    var summary = open.querySelector('summary');
    if (summary) summary.focus();
  });
})();
