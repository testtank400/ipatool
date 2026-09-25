(() => {
  const $ = (id) => document.getElementById(id);

  function go() {
    if (!window.go || !window.go.main || !window.go.main.App) {
      throw new Error("Wails bindings not ready");
    }
    return window.go.main.App;
  }

  function setStatus(el, msg, kind) {
    el.textContent = msg || "";
    el.classList.remove("error", "ok");
    if (kind) el.classList.add(kind);
  }

  function switchTab(name) {
    document.querySelectorAll(".tab").forEach((t) => {
      t.classList.toggle("active", t.dataset.tab === name);
    });
    document.querySelectorAll(".panel").forEach((p) => {
      p.classList.toggle("active", p.id === "panel-" + name);
    });
  }

  document.querySelectorAll(".tab").forEach((btn) => {
    btn.addEventListener("click", () => switchTab(btn.dataset.tab));
  });

  async function refreshAccount() {
    const badge = $("account-badge");
    try {
      const passphrase = $("keychainPassphrase").value;
      if (passphrase) {
        go().SetKeychainPassphrase(passphrase);
      }
      const info = await go().AccountInfo();
      badge.textContent = info.name ? `${info.name} <${info.email}>` : info.email || "Signed in";
    } catch (_) {
      badge.textContent = "Not signed in";
    }
  }

  $("login-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const btn = $("login-btn");
    const status = $("login-status");
    btn.disabled = true;
    setStatus(status, "Signing in (first login / SAP can take several minutes).");
    try {
      const result = await go().Login(
        $("email").value.trim(),
        $("password").value,
        $("authCode").value.trim(),
        $("keychainPassphrase").value
      );
      setStatus(status, `Signed in as ${result.account.name || result.account.email}`, "ok");
      await refreshAccount();
    } catch (err) {
      setStatus(status, String(err), "error");
    } finally {
      btn.disabled = false;
    }
  });

  $("revoke-btn").addEventListener("click", async () => {
    const status = $("login-status");
    try {
      const passphrase = $("keychainPassphrase").value;
      if (passphrase) go().SetKeychainPassphrase(passphrase);
      await go().Revoke();
      setStatus(status, "Credentials revoked", "ok");
      await refreshAccount();
    } catch (err) {
      setStatus(status, String(err), "error");
    }
  });

  function resetVersionSelect(preserveLatestLabel) {
    const sel = $("dl-version");
    const prev = sel.value;
    sel.innerHTML = "";
    const latest = document.createElement("option");
    latest.value = "";
    latest.textContent = preserveLatestLabel || "Latest";
    sel.appendChild(latest);
    return prev;
  }

  // CLI defaults: page 1, max-results 0 (=all IDs). With --resolve you must page;
  // DefaultVersionResolvePageSize is 10.
  const RESOLVE_PAGE_SIZE = 10;
  let versionsPage = 1;
  let versionsTotalCount = 0;
  let versionsMaxResults = 0;
  let versionsLoading = false;

  function currentResolve() {
    return !!$("dl-resolve-versions").checked;
  }

  function pageSizeForResolve(resolve) {
    // Never resolve the full history in one shot.
    return resolve ? RESOLVE_PAGE_SIZE : 0;
  }

  function updatePager(page, maxResults, totalCount, resolve) {
    const pager = $("version-pager");
    const label = $("version-page-label");
    const prev = $("dl-versions-prev");
    const next = $("dl-versions-next");
    versionsPage = page;
    versionsTotalCount = totalCount || 0;
    versionsMaxResults = maxResults;

    const paging = resolve && maxResults > 0;
    pager.hidden = !paging;
    if (!paging) {
      prev.disabled = true;
      next.disabled = true;
      label.textContent = "Page 1";
      return;
    }

    const totalPages = Math.max(1, Math.ceil(versionsTotalCount / maxResults) || 1);
    label.textContent = `Page ${page} / ${totalPages}`;
    prev.disabled = page <= 1 || versionsLoading;
    next.disabled = page >= totalPages || versionsLoading;
  }

  function formatVersionLabel(v, isLatest) {
    const id = v.externalVersionID || "";
    let label = "";
    if (v.displayVersion) {
      label = v.displayVersion;
      if (v.releaseDate) {
        const d = String(v.releaseDate).slice(0, 10);
        label += ` (${d})`;
      }
      label += ` - ${id}`;
    } else if (v.error) {
      // Keep the ID selectable; surface the real App Store / resolve error.
      label = `${id} (${v.error})`;
    } else {
      label = id;
    }
    if (isLatest) label += " - latest";
    return label;
  }

  function populateVersions(result, resolve) {
    const sel = $("dl-version");
    const prev = resetVersionSelect(
      result.latestExternalVersionID
        ? `Latest (${result.latestExternalVersionID})`
        : "Latest"
    );

    const ids = result.externalVersionIdentifiers || [];
    const byID = {};
    (result.versions || []).forEach((v) => {
      byID[v.externalVersionID] = v;
    });

    ids.forEach((id) => {
      const opt = document.createElement("option");
      opt.value = id;
      const meta = byID[id] || { externalVersionID: id };
      opt.textContent = formatVersionLabel(meta, id === result.latestExternalVersionID);
      sel.appendChild(opt);
    });

    if (prev && [...sel.options].some((o) => o.value === prev)) {
      sel.value = prev;
    } else {
      sel.value = "";
    }

    const page = result.page || versionsPage || 1;
    const shown = ids.length;
    const total = result.totalCount || shown;
    const maxResults = resolve ? RESOLVE_PAGE_SIZE : 0;
    updatePager(page, maxResults, total, resolve);

    let msg = `${shown} version(s)`;
    if (total > shown) msg += ` of ${total}`;
    if (resolve) {
      const detailed = (result.versions || []).filter((v) => v.displayVersion).length;
      msg += detailed ? ` - ${detailed} with details` : " - IDs only";
    }
    if (result.resolveError) {
      setStatus($("version-status"), `${msg}. Details failed: ${result.resolveError}`, "error");
    } else {
      setStatus($("version-status"), msg, "ok");
    }
  }

  async function loadVersions(opts) {
    const status = $("version-status");
    const appID = parseInt($("dl-app-id").value, 10) || 0;
    const bundleID = $("dl-bundle-id").value.trim();
    if (!appID && !bundleID) {
      setStatus(status, "Enter an app ID or bundle ID first", "error");
      return;
    }
    if (versionsLoading) return;
    versionsLoading = true;

    const resolve = !!(opts && opts.resolve != null ? opts.resolve : currentResolve());
    if (opts && opts.page != null) {
      versionsPage = opts.page;
    } else if (opts && opts.resetPage) {
      versionsPage = 1;
    }
    if (versionsPage < 1) versionsPage = 1;

    const maxResults = pageSizeForResolve(resolve);
    const page = resolve ? versionsPage : 1;

    const btn = $("dl-load-versions");
    btn.disabled = true;
    updatePager(page, maxResults, versionsTotalCount, resolve);
    setStatus(
      status,
      resolve
        ? `Loading version history page ${page} (resolving up to ${maxResults} details)...`
        : "Loading version history (IDs only)..."
    );
    try {
      const passphrase = $("keychainPassphrase").value;
      if (passphrase) go().SetKeychainPassphrase(passphrase);
      // IDs-only: page=1, maxResults=0 (all). Details: page N, maxResults=10.
      const result = await go().ListVersions(
        appID,
        bundleID,
        resolve,
        page,
        maxResults,
        $("dl-platform").value
      );
      populateVersions(result, resolve);
    } catch (err) {
      resetVersionSelect("Latest");
      updatePager(1, 0, 0, false);
      let msg = String(err);
      // Wails may prefix with "Error: "; keep the step tag (list history / resolve versions).
      if (msg.startsWith("Error: ")) msg = msg.slice(7);
      setStatus(status, msg, "error");
    } finally {
      versionsLoading = false;
      btn.disabled = false;
      updatePager(versionsPage, versionsMaxResults, versionsTotalCount, currentResolve());
    }
  }

  $("dl-load-versions").addEventListener("click", () => loadVersions({ resetPage: true }));
  $("dl-resolve-versions").addEventListener("change", () => {
    const appID = parseInt($("dl-app-id").value, 10) || 0;
    const bundleID = $("dl-bundle-id").value.trim();
    if (appID || bundleID) loadVersions({ resetPage: true });
  });
  $("dl-versions-prev").addEventListener("click", () => {
    if (versionsPage <= 1) return;
    loadVersions({ page: versionsPage - 1 });
  });
  $("dl-versions-next").addEventListener("click", () => {
    loadVersions({ page: versionsPage + 1 });
  });

  $("dl-version").addEventListener("change", () => {
    const v = $("dl-version").value.trim();
    if (v) $("dl-external-version-id").value = v;
  });
  $("dl-browse").addEventListener("click", async () => {
    try {
      const path = await go().SelectOutputPath();
      if (path) $("dl-output").value = path;
    } catch (err) {
      setStatus($("download-status"), String(err), "error");
    }
  });

  $("search-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const status = $("search-status");
    const list = $("search-results");
    list.innerHTML = "";
    setStatus(status, "Searching.");
    try {
      const passphrase = $("keychainPassphrase").value;
      if (passphrase) go().SetKeychainPassphrase(passphrase);
      const limit = parseInt($("search-limit").value, 10) || 10;
      const result = await go().Search(
        $("search-term").value.trim(),
        limit,
        $("search-platform").value
      );
      setStatus(status, `${result.count} result(s)`, "ok");
      (result.apps || []).forEach((app) => {
        const li = document.createElement("li");
        li.innerHTML = `<div><strong>${escapeHtml(app.name)}</strong>
          <div class="meta">${escapeHtml(app.bundleID)} · id ${app.id} · v${escapeHtml(app.version)} · $${app.price}</div></div>`;
        const use = document.createElement("button");
        use.type = "button";
        use.textContent = "Use";
        use.className = "secondary";
        use.addEventListener("click", () => {
          $("dl-app-id").value = app.id;
          $("dl-bundle-id").value = app.bundleID;
          $("dl-platform").value = $("search-platform").value;
          resetVersionSelect("Latest");
          $("dl-external-version-id").value = "";
          switchTab("download");
          loadVersions({ resetPage: true });
        });
        li.appendChild(use);
        list.appendChild(li);
      });
    } catch (err) {
      setStatus(status, String(err), "error");
    }
  });

  const bar = $("progress-bar");
  const dlStatus = $("download-status");

  function onReady(fn) {
    if (window.runtime && window.runtime.EventsOn) {
      fn();
      return;
    }
    window.addEventListener("DOMContentLoaded", () => setTimeout(fn, 50));
    setTimeout(fn, 200);
  }

  onReady(() => {
    if (!window.runtime || !window.runtime.EventsOn) return;
    window.runtime.EventsOn("download:start", () => {
      bar.classList.add("indeterminate");
      bar.style.width = "40%";
      setStatus(dlStatus, "Downloading.");
    });
    window.runtime.EventsOn("download:done", (data) => {
      bar.classList.remove("indeterminate");
      bar.style.width = "100%";
      const path = data && data.destinationPath ? data.destinationPath : "";
      setStatus(dlStatus, path ? `Saved to ${path}` : "Download complete", "ok");
    });
    window.runtime.EventsOn("download:error", (data) => {
      bar.classList.remove("indeterminate");
      bar.style.width = "0%";
      setStatus(dlStatus, (data && data.error) || "Download failed", "error");
    });
    refreshAccount().catch(() => {});
  });

  function resolveExternalVersionID() {
    const manual = $("dl-external-version-id").value.trim();
    const fromList = $("dl-version").value.trim();
    if (manual) {
      if (!/^\d+$/.test(manual)) {
        return { error: "Enter a numeric external version ID (e.g. 890891604), or leave blank for latest." };
      }
      return { id: manual };
    }
    return { id: fromList };
  }

  $("download-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const btn = $("download-btn");
    const resolved = resolveExternalVersionID();
    if (resolved.error) {
      bar.classList.remove("indeterminate");
      bar.style.width = "0%";
      setStatus(dlStatus, resolved.error, "error");
      return;
    }
    btn.disabled = true;
    bar.classList.add("indeterminate");
    bar.style.width = "40%";
    const verNote = resolved.id ? ` (version ${resolved.id})` : " (latest)";
    setStatus(dlStatus, "Starting download" + verNote + ".");
    try {
      const passphrase = $("keychainPassphrase").value;
      if (passphrase) go().SetKeychainPassphrase(passphrase);
      const appID = parseInt($("dl-app-id").value, 10) || 0;
      const result = await go().Download(
        appID,
        $("dl-bundle-id").value.trim(),
        $("dl-output").value.trim(),
        resolved.id,
        $("dl-platform").value
      );
      bar.classList.remove("indeterminate");
      bar.style.width = "100%";
      setStatus(dlStatus, `Saved to ${result.destinationPath}`, "ok");
    } catch (err) {
      bar.classList.remove("indeterminate");
      bar.style.width = "0%";
      setStatus(dlStatus, String(err), "error");
    } finally {
      btn.disabled = false;
    }
  });

  function escapeHtml(s) {
    return String(s == null ? "" : s)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }
})();
