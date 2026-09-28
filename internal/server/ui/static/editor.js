// The browser editor's conveniences (SPEC-016 SD-1): Ctrl-S or Cmd-S saves,
// a button copies the text, and leaving with unsaved text asks first. The
// editor works without any of it.
//
// Every listener is delegated on the document, like dialog.js, so an editor
// that arrives by a boosted navigation needs no set-up, and a page without
// one pays nothing.
(function () {
  var dirty = false;

  document.addEventListener('input', function (e) {
    if (e.target.closest && e.target.closest('form[data-editor]')) dirty = true;
  });

  // A save is not leaving.
  document.addEventListener('submit', function (e) {
    if (e.target.matches && e.target.matches('form[data-editor]')) dirty = false;
  });

  // Ctrl-S or Cmd-S presses Save (FR-1.4).
  document.addEventListener('keydown', function (e) {
    if ((e.ctrlKey || e.metaKey) && !e.altKey && (e.key === 's' || e.key === 'S')) {
      var save = document.getElementById('ed-save');
      if (!save) return;
      e.preventDefault();
      save.click();
    }
  });

  // Copy your text (SD-7).
  document.addEventListener('click', function (e) {
    var btn = e.target.closest && e.target.closest('[data-copy-target]');
    if (!btn) return;
    var box = document.getElementById(btn.getAttribute('data-copy-target'));
    if (!box) return;
    var label = btn.querySelector('[data-copy-label]') || btn;
    var done = function () {
      label.textContent = 'Copied';
      setTimeout(function () { label.textContent = 'Copy your text'; }, 2000);
    };
    if (navigator.clipboard && navigator.clipboard.writeText) {
      navigator.clipboard.writeText(box.value).then(done, function () {
        box.select(); document.execCommand('copy'); done();
      });
    } else {
      box.select(); document.execCommand('copy'); done();
    }
  });

  // Leaving with unsaved text asks first. Boosted links never unload the
  // page, so they are checked here as well as on unload.
  document.addEventListener('click', function (e) {
    if (!dirty) return;
    var a = e.target.closest && e.target.closest('a[href]');
    if (!a || a.target === '_blank') return;
    if (!window.confirm('Your text isn’t saved. Leave the editor and lose it?')) {
      e.preventDefault();
      e.stopImmediatePropagation();
    } else {
      dirty = false;
    }
  }, true);

  // Any other boosted request that would replace the page (a form
  // elsewhere, say) asks too. The editor's own save and preview don't.
  document.addEventListener('htmx:confirm', function (e) {
    if (!dirty || !e.detail || !e.detail.elt) return;
    var elt = e.detail.elt;
    if (elt.closest && (elt.closest('form[data-editor]') || elt.closest('#ed-preview') || elt.closest('#edit-fresh'))) return;
    if (elt.tagName === 'A') return; // links were asked about on click
    if (!elt.hasAttribute('hx-boost') && !(elt.closest && elt.closest('[hx-boost="true"]'))) return;
    if (!window.confirm('Your text isn\u2019t saved. Leave the editor and lose it?')) {
      e.preventDefault();
    } else {
      dirty = false;
    }
  });

  window.addEventListener('beforeunload', function (e) {
    if (dirty) { e.preventDefault(); e.returnValue = ''; }
  });

  // A page swapped in by htmx starts clean.
  document.addEventListener('htmx:afterSettle', function (e) {
    if (e.target === document.body) dirty = false;
  });
})();
