(function () {
  function storedTheme() {
    try {
      var pref = localStorage.getItem("kafclaw-theme");
      return pref === "dark" || pref === "light" ? pref : null;
    } catch (e) {
      return null;
    }
  }

  function activeTheme() {
    var attr = document.documentElement.getAttribute("data-theme");
    if (attr === "dark" || attr === "light") return attr;

    var pref = storedTheme();
    if (pref) return pref;
    if (window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches) return "dark";
    return "light";
  }

  function applyTheme(val) {
    document.documentElement.setAttribute("data-theme", val);
    try {
      localStorage.setItem("kafclaw-theme", val);
    } catch (e) {}
  }

  function setIcon(btn, theme) {
    if (theme === "dark") {
      btn.innerHTML =
        '<svg viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d="M6.76 4.84l-1.8-1.79-1.41 1.41 1.79 1.8 1.42-1.42zM1 13h3v-2H1v2zm10 9h2v-3h-2v3zm9.66-17.54l-1.41-1.41-1.8 1.79 1.42 1.42 1.79-1.8zM17.24 19.16l1.8 1.79 1.41-1.41-1.79-1.8-1.42 1.42zM20 11v2h3v-2h-3zM12 6a6 6 0 100 12 6 6 0 000-12zm0-5h-2v3h2V1z"/></svg>';
      btn.title = "Switch to light mode";
    } else {
      btn.innerHTML =
        '<svg viewBox="0 0 24 24" aria-hidden="true"><path fill="currentColor" d="M21.75 15.5A9 9 0 1112.5 2.25a7 7 0 109.25 13.25z"/></svg>';
      btn.title = "Switch to dark mode";
    }
  }

  function setup() {
    var btn = document.getElementById("theme-toggle-btn");
    if (!btn) return;

    setIcon(btn, activeTheme());
    function handleToggle() {
      var now = activeTheme();
      var next = now === "dark" ? "light" : "dark";
      applyTheme(next);
      setIcon(btn, next);
    }

    btn.onclick = handleToggle;
    btn.addEventListener("click", handleToggle);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", setup);
  } else {
    setup();
  }
})();
