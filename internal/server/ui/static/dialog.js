// The only bespoke script in the app. The few things the design system
// genuinely cannot do in CSS, and nothing else.
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

  // --- 1a. An editor loaded by HTMX opens itself (SPEC-010 FR-6) -----------
  //
  // The milestone and roadmap editors arrive as a <dialog data-autoshow> in
  // #modal-slot. htmx:load fires for every piece of content HTMX inserts, so
  // this one listener opens any such dialog however it arrived.
  document.addEventListener('htmx:load', function (e) {
    var el = e.detail && e.detail.elt;
    if (!el || !el.querySelector) return;
    var dlg = el.matches('dialog[data-autoshow]') ? el : el.querySelector('dialog[data-autoshow]');
    if (dlg && !dlg.open && typeof dlg.showModal === 'function') dlg.showModal();
  });

  // --- 1b. Closing an editor after a change refreshes the page -------------
  //
  // Forms inside an editor swap only the editor, so the page underneath is
  // stale once something changed. The server marks the editor data-changed
  // after a successful change; closing it by any means (Done, Escape, the
  // backdrop) then reloads, and closing without a change just closes. The
  // close event does not bubble, hence the capture listener.
  document.addEventListener('close', function (e) {
    var dlg = e.target;
    if (!dlg || !dlg.matches || !dlg.matches('dialog[data-refresh-on-close]')) return;
    if (dlg.querySelector('[data-changed="true"]')) {
      window.location.reload();
    } else if (dlg.parentNode) {
      dlg.parentNode.removeChild(dlg);
    }
  }, true);

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

  // --- 3. Arrow keys in the project-structure rail -------------------------
  //
  // The rail scopes rather than nests, so drilling in and stepping back out are
  // the two moves it has. Without these it is mouse-only: the drill arrow is a
  // link you can tab to, but "go in / go up" should be on the arrow keys where
  // anyone who has used a tree will look for them. (Design round 5 asks for it.)
  //
  // Both are ordinary navigations, because the scope follows the page — there is
  // no rail state to change independently.
  document.addEventListener('keydown', function (e) {
    if (e.key !== 'ArrowRight' && e.key !== 'ArrowLeft') return;
    var el = document.activeElement;
    if (!el || !el.closest) return;
    var row = el.closest('.tree-row');
    var inRail = row || el.closest('.tree-scope, .tree-path');
    if (!inRail) return;

    if (e.key === 'ArrowRight') {
      // Into the focused row, if it has an inside to go to.
      var into = row && row.querySelector('.tree-into');
      if (into) { e.preventDefault(); into.click(); }
      return;
    }
    // Up one level: the nearest ancestor in the trail, else the scope itself.
    var crumbs = document.querySelectorAll('.tree-path .tree-crumb');
    var up = crumbs.length ? crumbs[crumbs.length - 1]
                           : document.querySelector('.tree-scope--nested');
    if (up) { e.preventDefault(); up.click(); }
  });
})();
