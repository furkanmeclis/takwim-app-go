/*
 * Takwim booking embed loader (TEC-908).
 *
 * Inline booking area:
 *   <div data-takwim-slug="your-business"></div>
 * "Book" button opening a window:
 *   <button type="button" data-takwim-open="your-business">Randevu al</button>
 * Then, once per page:
 *   <script async src="https://takwim.link/embed.js"></script>
 *
 * Optional attributes on either element: data-takwim-view="full" (profile,
 * tabs) and the pre-selection data-takwim-branch, data-takwim-service,
 * data-takwim-staff (uuids) and data-takwim-date (YYYY-MM-DD).
 * Script API: Takwim.open({ slug, view, branch, service, staff, date }),
 * Takwim.close(), Takwim.scan().
 *
 * The frame only works on the sites listed in the business's embed
 * settings. Messages from the frame are accepted only from the Takwim
 * origin and from the frame they name; they carry the height and nothing
 * about the customer.
 */
(function () {
  "use strict";

  if (window.Takwim && window.Takwim.version) {
    window.Takwim.scan();
    return;
  }

  var script =
    document.currentScript ||
    document.querySelector('script[src$="/embed.js"]');
  if (!script || !script.src) return;
  var BASE = new URL(script.src, window.location.href).origin;
  var SOURCE = "takwim-embed";
  var VERSION = 1;
  var PRESELECT = ["branch", "service", "staff", "date"];
  var UUID_RE =
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
  var DATE_RE = /^\d{4}-\d{2}-\d{2}$/;
  var SLUG_RE = /^[a-z0-9][a-z0-9-]{0,63}$/i;
  var MIN_HEIGHT = 240;
  var MAX_HEIGHT = 20000;

  var frames = {};
  var counter = 0;
  var modal = null;

  var english = /^en\b/i.test(document.documentElement.lang || "");
  var TEXT = english
    ? { title: "Online booking", close: "Close" }
    : { title: "Online randevu", close: "Kapat" };

  function injectStyle() {
    if (document.getElementById("takwim-embed-style")) return;
    var style = document.createElement("style");
    style.id = "takwim-embed-style";
    style.textContent =
      ".takwim-embed-frame{display:block;width:100%;min-width:280px;border:0;background:transparent;color-scheme:normal}" +
      ".takwim-embed-overlay{position:fixed;inset:0;z-index:2147483000;display:flex;align-items:center;justify-content:center;padding:16px;background:rgba(15,23,42,.55)}" +
      ".takwim-embed-dialog{position:relative;display:flex;flex-direction:column;width:100%;max-width:760px;height:min(880px,calc(100vh - 32px));background:#fff;border-radius:12px;overflow:hidden;box-shadow:0 20px 50px rgba(0,0,0,.3)}" +
      ".takwim-embed-bar{display:flex;justify-content:flex-end;align-items:center;height:44px;padding:0 6px;border-bottom:1px solid rgba(0,0,0,.08);background:#fff}" +
      ".takwim-embed-close{width:36px;height:36px;border:0;border-radius:8px;background:transparent;font:24px/1 system-ui,sans-serif;color:#111;cursor:pointer}" +
      ".takwim-embed-close:focus-visible{outline:2px solid #2563eb;outline-offset:2px}" +
      ".takwim-embed-dialog .takwim-embed-frame{flex:1;height:100%}" +
      "@media (max-width:640px){.takwim-embed-overlay{padding:0}.takwim-embed-dialog{max-width:none;height:100%;border-radius:0}}";
    document.head.appendChild(style);
  }

  function attr(el, name) {
    var value = el.getAttribute("data-takwim-" + name);
    return value ? value.trim() : "";
  }

  function optionsOf(el) {
    var o = {
      slug: attr(el, "slug") || attr(el, "open"),
      view: attr(el, "view"),
      title: attr(el, "title"),
    };
    for (var i = 0; i < PRESELECT.length; i++) {
      o[PRESELECT[i]] = attr(el, PRESELECT[i]);
    }
    return o;
  }

  function frameURL(o, id, isModal) {
    var url = new URL(BASE + "/embed/" + encodeURIComponent(o.slug));
    url.searchParams.set("frame", id);
    url.searchParams.set("origin", window.location.origin);
    if (o.view === "full") url.searchParams.set("view", "full");
    if (isModal) url.searchParams.set("mode", "modal");
    for (var i = 0; i < PRESELECT.length; i++) {
      var key = PRESELECT[i];
      var value = o[key] ? String(o[key]) : "";
      var ok = key === "date" ? DATE_RE.test(value) : UUID_RE.test(value);
      if (ok) url.searchParams.set(key, value);
    }
    return url.toString();
  }

  function createFrame(o, isModal) {
    if (!o || !SLUG_RE.test(o.slug || "")) return null;
    injectStyle();
    counter += 1;
    var id = "tk" + counter + Math.random().toString(36).slice(2, 8);
    var iframe = document.createElement("iframe");
    iframe.className = "takwim-embed-frame";
    iframe.title = o.title || TEXT.title;
    iframe.src = frameURL(o, id, isModal);
    iframe.referrerPolicy = "strict-origin-when-cross-origin";
    if (!isModal) {
      iframe.loading = "lazy";
      iframe.style.height = "640px";
    }
    frames[id] = { iframe: iframe, inline: !isModal };
    return iframe;
  }

  function forget(iframe) {
    for (var id in frames) {
      if (frames[id].iframe === iframe) delete frames[id];
    }
  }

  function mount(el) {
    if (el.getAttribute("data-takwim-mounted")) return;
    var iframe = createFrame(optionsOf(el), false);
    if (!iframe) return;
    el.setAttribute("data-takwim-mounted", "1");
    el.appendChild(iframe);
  }

  function scan() {
    var nodes = document.querySelectorAll("[data-takwim-slug]");
    for (var i = 0; i < nodes.length; i++) mount(nodes[i]);
  }

  function onKey(event) {
    if (event.key === "Escape") close();
  }

  function open(o, opener) {
    close();
    var iframe = createFrame(o, true);
    if (!iframe) return;
    var overlay = document.createElement("div");
    overlay.className = "takwim-embed-overlay";
    var dialog = document.createElement("div");
    dialog.className = "takwim-embed-dialog";
    dialog.setAttribute("role", "dialog");
    dialog.setAttribute("aria-modal", "true");
    dialog.setAttribute("aria-label", iframe.title);
    var bar = document.createElement("div");
    bar.className = "takwim-embed-bar";
    var button = document.createElement("button");
    button.type = "button";
    button.className = "takwim-embed-close";
    button.setAttribute("aria-label", TEXT.close);
    button.textContent = "×";
    button.addEventListener("click", close);
    bar.appendChild(button);
    dialog.appendChild(bar);
    dialog.appendChild(iframe);
    overlay.appendChild(dialog);
    overlay.addEventListener("click", function (event) {
      if (event.target === overlay) close();
    });
    modal = {
      overlay: overlay,
      iframe: iframe,
      opener: opener || document.activeElement,
      overflow: document.documentElement.style.overflow,
    };
    document.documentElement.style.overflow = "hidden";
    document.body.appendChild(overlay);
    document.addEventListener("keydown", onKey);
    button.focus();
  }

  function close() {
    if (!modal) return;
    var current = modal;
    modal = null;
    document.removeEventListener("keydown", onKey);
    document.documentElement.style.overflow = current.overflow;
    forget(current.iframe);
    if (current.overlay.parentNode) {
      current.overlay.parentNode.removeChild(current.overlay);
    }
    if (current.opener && typeof current.opener.focus === "function") {
      current.opener.focus();
    }
  }

  window.addEventListener("message", function (event) {
    if (event.origin !== BASE) return;
    var data = event.data;
    if (
      !data ||
      typeof data !== "object" ||
      data.source !== SOURCE ||
      data.v !== VERSION ||
      typeof data.frame !== "string" ||
      typeof data.type !== "string"
    ) {
      return;
    }
    var entry = frames[data.frame];
    if (!entry || event.source !== entry.iframe.contentWindow) return;
    if (data.type === "resize" && entry.inline) {
      var height = Number(data.height);
      if (!isFinite(height)) return;
      height = Math.min(Math.max(Math.round(height), MIN_HEIGHT), MAX_HEIGHT);
      entry.iframe.style.height = height + "px";
    } else if (data.type === "scroll_top" && entry.inline) {
      var top = entry.iframe.getBoundingClientRect().top;
      if (top < 0 || top > window.innerHeight * 0.5) {
        entry.iframe.scrollIntoView({ behavior: "smooth", block: "start" });
      }
    }
  });

  document.addEventListener("click", function (event) {
    var target = event.target;
    var el =
      target && target.closest ? target.closest("[data-takwim-open]") : null;
    if (!el) return;
    event.preventDefault();
    open(optionsOf(el), el);
  });

  window.Takwim = {
    version: VERSION,
    open: function (o) {
      open(o || {}, document.activeElement);
    },
    close: close,
    scan: scan,
  };

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", scan);
  } else {
    scan();
  }
})();
