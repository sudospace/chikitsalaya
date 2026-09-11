// Delegated listeners so behavior survives htmx swaps (e.g. the ICD-10
// search results fragment). Handlers below use an element's .form
// property (not .closest) since row fields are linked to their <form> via
// the form="id" attribute, not DOM nesting.
document.addEventListener("click", function (e) {
	var btn = e.target.closest("[data-icd10-pick]");
	if (!btn) return;

	var form = document.getElementById("diagnosis-form");
	if (!form) return;

	var code = btn.getAttribute("data-code");
	var description = btn.getAttribute("data-description");

	var hidden = form.querySelector('input[name="icd10_code"]');
	if (hidden) hidden.value = code;

	var descField = document.querySelector('[form="diagnosis-form"][name="description"]');
	if (descField) descField.value = description;

	var searchField = document.querySelector('[form="diagnosis-form"][name="icd10_search"]');
	if (searchField) searchField.value = code + " — " + description;

	var results = document.getElementById("icd10-results");
	if (results) results.innerHTML = "";
});

// Finds the hidden catalog-id input for a visible field: checks the
// field's own <tr> first (multi-row tables), then falls back to its
// <form> (a single add-row field keeps its hidden input there instead).
function findScopedHidden(input, hiddenName) {
	var row = input.closest("tr");
	var hidden = row && row.querySelector('input[name="' + hiddenName + '"]');
	if (hidden) return hidden;
	var form = input.form;
	return (form && form.querySelector('input[name="' + hiddenName + '"]')) || null;
}

// Mobile navbar toggle: the header's <nav> collapses below 860px (see
// app.css) and opens as a dropdown panel via this class toggle.
document.addEventListener("click", function (e) {
	var btn = e.target.closest("[data-nav-toggle]");
	if (!btn) return;
	var nav = document.getElementById(btn.getAttribute("aria-controls"));
	if (!nav) return;
	var open = nav.classList.toggle("is-open");
	btn.setAttribute("aria-expanded", open ? "true" : "false");
});

// Generic nav dropdown (Catalog/Setup, profile menu): opens on click,
// closing any other open one first; closes on outside click or Escape.
function closeAllNavDropdowns(except) {
	document.querySelectorAll(".nav-dropdown-toggle[aria-expanded=\"true\"]").forEach(function (btn) {
		if (btn === except) return;
		btn.setAttribute("aria-expanded", "false");
		var menu = btn.nextElementSibling;
		if (menu) menu.hidden = true;
	});
}

document.addEventListener("click", function (e) {
	var btn = e.target.closest("[data-dropdown-toggle]");
	if (btn) {
		var menu = btn.nextElementSibling;
		if (!menu) return;
		var willOpen = menu.hidden;
		closeAllNavDropdowns(btn);
		menu.hidden = !willOpen;
		btn.setAttribute("aria-expanded", willOpen ? "true" : "false");
		return;
	}
	// Any other click (including inside an open menu, e.g. picking a
	// link) closes whatever's open.
	if (!e.target.closest(".nav-dropdown-menu")) closeAllNavDropdowns();
});

document.addEventListener("keydown", function (e) {
	if (e.key === "Escape") closeAllNavDropdowns();
});

// Generic "+ Add line": clones a <template> row into a target container.
document.addEventListener("click", function (e) {
	var btn = e.target.closest("[data-add-row]");
	if (!btn) return;
	var tmpl = document.getElementById(btn.getAttribute("data-template"));
	var target = document.getElementById(btn.getAttribute("data-target"));
	if (!tmpl || !target || !("content" in tmpl)) return;
	target.appendChild(tmpl.content.cloneNode(true));
});

// New-user form: the practitioner-record section only applies to a Doctor login.
document.addEventListener("change", function (e) {
	var select = e.target.closest("[data-role-select]");
	if (!select) return;
	var section = document.getElementById(select.getAttribute("data-role-panel-target"));
	if (!section) return;
	var isDoctor = select.value === "doctor";
	section.hidden = !isDoctor;
	section.querySelectorAll("input, select").forEach(function (field) {
		if (field === select) return;
		field.disabled = !isDoctor;
	});
});

// New-user form: "Create a new practitioner" / "Link an existing one" —
// same shape as the patient-mode toggle above.
document.addEventListener("change", function (e) {
	var radio = e.target.closest("[data-practitioner-mode]");
	if (!radio) return;
	document.querySelectorAll("[data-practitioner-panel]").forEach(function (panel) {
		var active = panel.getAttribute("data-practitioner-panel") === radio.value;
		panel.hidden = !active;
		panel.querySelectorAll("input, select, textarea").forEach(function (field) {
			field.disabled = !active;
		});
	});
});

// Letterhead mode toggle: same shape as the patient-mode toggle above.
document.addEventListener("change", function (e) {
	var radio = e.target.closest("[data-letterhead-mode]");
	if (!radio) return;
	document.querySelectorAll("[data-letterhead-panel]").forEach(function (panel) {
		var active = panel.getAttribute("data-letterhead-panel") === radio.value;
		panel.hidden = !active;
		panel.querySelectorAll("input, select, textarea").forEach(function (field) {
			field.disabled = !active;
		});
	});
});

// Catalog picker fields share one floating results container per field
// *type*, anchored via position:fixed under the active input (fixed, not
// absolute, so it isn't clipped by the table's own overflow-x:auto).
// lastCatalogSearchInput tracks which field is currently showing results.
var lastCatalogSearchInput = null;

function positionDropdown(input, results) {
	var rect = input.getBoundingClientRect();
	results.style.left = rect.left + "px";
	results.style.top = rect.bottom + 4 + "px";
	results.style.width = rect.width + "px";
	results.hidden = false;
}

function hideDropdown(results) {
	if (!results) return;
	results.hidden = true;
	results.innerHTML = "";
}

// A fixed-position dropdown doesn't move with page scroll, so just close
// it, like a native picker losing its anchor.
window.addEventListener(
	"scroll",
	function (e) {
		if (!lastCatalogSearchInput) return;
		var results = document.getElementById(lastCatalogSearchInput.getAttribute("data-results"));
		// A scroll inside the dropdown's own option list must not close it —
		// only a scroll elsewhere on the page should.
		if (results && results.contains(e.target)) return;
		hideDropdown(results);
		lastCatalogSearchInput = null;
	},
	true
);

// Live search-as-you-type, replacing a preloaded <datalist> (doesn't
// scale, inconsistent on mobile). Starts at 3 characters, matching the
// server's own minimum.
document.addEventListener("input", function (e) {
	var input = e.target.closest("[data-live-search]");
	if (!input) return;

	var resultsID = input.getAttribute("data-results");
	var results = resultsID && document.getElementById(resultsID);
	if (!results) return;

	// Typing again may invalidate the previous pick — clear the link until a fresh pick re-establishes it.
	var hiddenName = input.getAttribute("data-hidden-id");
	var hidden = hiddenName && findScopedHidden(input, hiddenName);
	if (hidden) hidden.value = "";

	clearTimeout(input._searchTimer);
	var value = input.value.trim();
	if (value.length < 3) {
		hideDropdown(results);
		return;
	}

	var endpoint = input.getAttribute("data-endpoint");
	input._searchTimer = setTimeout(function () {
		fetch(endpoint + "?q=" + encodeURIComponent(value))
			.then(function (resp) { return resp.ok ? resp.text() : ""; })
			.then(function (html) {
				// The response always wraps in a container, even with zero real
				// hits — a "+ Add" affordance renders there instead, which is
				// itself a valid "show the dropdown" state, not an empty one.
				results.innerHTML = html;
				lastCatalogSearchInput = input;
				positionDropdown(input, results);
			})
			.catch(function (err) {
				console.error(err);
			});
	}, 300);
});

// "+ Add" affordance inside a live-search dropdown (zero real hits): POST
// the typed value, swap in the response (now a real, pickable hit), then
// auto-apply it — no second click needed.
document.addEventListener("click", function (e) {
	var btn = e.target.closest("[data-quick-add-inline]");
	if (!btn || !lastCatalogSearchInput) return;

	var input = lastCatalogSearchInput;
	var endpoint = btn.getAttribute("data-endpoint");
	var fieldName = btn.getAttribute("data-field-name");
	var value = btn.getAttribute("data-value");
	if (!endpoint || !fieldName) return;

	btn.disabled = true;
	var body = new URLSearchParams();
	body.set(fieldName, value);

	fetch(endpoint, {
		method: "POST",
		headers: { "Content-Type": "application/x-www-form-urlencoded" },
		body: body.toString(),
	})
		.then(function (resp) {
			if (!resp.ok) throw new Error("quick-add failed: " + resp.status);
			return resp.text();
		})
		.then(function (html) {
			var results = document.getElementById(input.getAttribute("data-results"));
			if (!results) return;
			results.innerHTML = html;
			var pick = results.querySelector("[data-pick-catalog]");
			if (pick) pick.click();
		})
		.catch(function (err) {
			console.error(err);
			alert("Couldn't add that to the catalog. Please try again.");
		})
		.finally(function () {
			btn.disabled = false;
		});
});

document.addEventListener("click", function (e) {
	var btn = e.target.closest("[data-pick-catalog]");
	if (!btn || !lastCatalogSearchInput) return;

	var input = lastCatalogSearchInput;
	input.value = btn.getAttribute("data-value");
	var hiddenName = input.getAttribute("data-hidden-id");
	var hidden = hiddenName && findScopedHidden(input, hiddenName);
	if (hidden) hidden.value = btn.getAttribute("data-id");

	hideDropdown(document.getElementById(input.getAttribute("data-results")));
	lastCatalogSearchInput = null;

	maybeAutoFillQuantity(input);
});

// Quantity auto-fill: once dosage and duration are both known, suggest
// frequency/day x days as a starting point (editable, never forced).
// Anything the parser can't confidently read ("As needed", "Ongoing")
// just leaves quantity alone.
function parseDailyFrequency(text) {
	text = (text || "").trim().toLowerCase();
	if (!text) return null;

	// "1-0-1", "1-1-1-1" (per-slot doses): count is the sum of the slots.
	if (/^\d+(-\d+){1,4}$/.test(text)) {
		var sum = text.split("-").reduce(function (total, part) {
			return total + (parseInt(part, 10) || 0);
		}, 0);
		return sum > 0 ? sum : null;
	}

	var everyHours = text.match(/every\s+(\d+)\s*hours?/);
	if (everyHours) {
		var hrs = parseInt(everyHours[1], 10);
		return hrs > 0 ? Math.round((24 / hrs) * 10) / 10 : null;
	}

	var timesADay = text.match(/(\d+)\s*times?\s*a?\s*day/);
	if (timesADay) return parseInt(timesADay[1], 10);

	if (/\bfour times\b|\bqid\b/.test(text)) return 4;
	if (/\bthrice\b|\btid\b/.test(text)) return 3;
	if (/\btwice\b|\bbid\b/.test(text)) return 2;
	if (/\bonce\b/.test(text)) return 1;

	return null;
}

function parseDurationDays(text) {
	var m = (text || "").trim().toLowerCase().match(/^(\d+)\s*(hour|day|week|month)s?$/);
	if (!m) return null;
	var n = parseInt(m[1], 10);
	var perUnit = { hour: 1 / 24, day: 1, week: 7, month: 30 };
	return n * perUnit[m[2]];
}

// Marks quantity "manual" once typed, so auto-fill won't overwrite it;
// clearing it opts back in.
document.addEventListener("input", function (e) {
	var qty = e.target.closest('input[name="quantity"]');
	if (!qty) return;
	qty.dataset.manual = qty.value.trim() === "" ? "" : "1";
});

function maybeAutoFillQuantity(fieldInThatRow) {
	var row = fieldInThatRow.closest("tr");
	if (!row) return;
	var qty = row.querySelector('input[name="quantity"]');
	if (!qty || qty.dataset.manual === "1") return;

	var dosage = row.querySelector('input[name="dosage_text"]');
	var duration = row.querySelector('input[name="duration_text"]');
	var perDay = dosage && parseDailyFrequency(dosage.value);
	var days = duration && parseDurationDays(duration.value);
	if (!perDay || !days) return;

	qty.value = Math.max(1, Math.ceil(perDay * days));
}

// Close a field's dropdown on blur, delayed so a click on a result
// (which blurs first) still registers.
document.addEventListener("focusout", function (e) {
	var input = e.target.closest("[data-live-search], [data-dropdown-search]");
	if (!input) return;
	var resultsID = input.getAttribute("data-results");
	setTimeout(function () {
		// The results container is shared across rows of the same field
		// type — if focus already moved into another row of that type, its
		// handler has already repainted this div, so don't clobber it.
		// Check the live focused element since the bookkeeping var can go
		// stale on a zero-match search.
		var active = document.activeElement;
		if (active && active !== input && active.getAttribute &&
			active.getAttribute("data-results") === resultsID) {
			return;
		}
		hideDropdown(resultsID && document.getElementById(resultsID));
		if (lastCatalogSearchInput === input) lastCatalogSearchInput = null;
	}, 200);
});

// Dosage/duration picklists: short lists already loaded as JSON (see each
// field's data-options-id), so no server round trip — empty input shows
// everything, typing narrows by substring. Picking reuses
// [data-pick-catalog] above.
function dropdownFieldOptions(input) {
	if (input._options) return input._options;
	var src = document.getElementById(input.getAttribute("data-options-id"));
	try {
		input._options = (src && JSON.parse(src.textContent)) || [];
	} catch (err) {
		input._options = [];
	}
	return input._options;
}

function showDropdownOptions(input) {
	var results = document.getElementById(input.getAttribute("data-results"));
	if (!results) return;

	var q = input.value.trim().toLowerCase();
	var matches = dropdownFieldOptions(input).filter(function (opt) {
		return opt.value.toLowerCase().indexOf(q) !== -1;
	});

	if (!matches.length) {
		// Nothing to add for an empty field (that's just "show everything but
		// there's nothing loaded yet"), but a typed value with no match gets
		// an inline "+ Add" offer instead of just closing silently.
		if (!q) {
			hideDropdown(results);
			return;
		}
		results.innerHTML = "";
		var addList = document.createElement("ul");
		addList.className = "result-list";
		var addLi = document.createElement("li");
		var addBtn = document.createElement("button");
		addBtn.type = "button";
		addBtn.className = "small outline";
		addBtn.setAttribute("data-quick-add-lookup", "");
		addBtn.textContent = '+ Add "' + input.value.trim() + '" as new ' + (input.getAttribute("data-lookup-category") || "value");
		addLi.appendChild(addBtn);
		addList.appendChild(addLi);
		results.appendChild(addList);
		lastCatalogSearchInput = input;
		positionDropdown(input, results);
		return;
	}

	results.innerHTML = "";
	var list = document.createElement("ul");
	list.className = "result-list";
	matches.forEach(function (opt) {
		var li = document.createElement("li");
		var btn = document.createElement("button");
		btn.type = "button";
		btn.className = "small outline";
		btn.setAttribute("data-pick-catalog", "");
		btn.setAttribute("data-id", opt.id);
		btn.setAttribute("data-value", opt.value);
		btn.textContent = opt.value;
		li.appendChild(btn);
		list.appendChild(li);
	});
	results.appendChild(list);

	lastCatalogSearchInput = input;
	positionDropdown(input, results);
}

document.addEventListener("focusin", function (e) {
	var input = e.target.closest("[data-dropdown-search]");
	if (!input) return;
	showDropdownOptions(input);
});

document.addEventListener("input", function (e) {
	var input = e.target.closest("[data-dropdown-search]");
	if (!input) return;
	var hiddenName = input.getAttribute("data-hidden-id");
	var hidden = hiddenName && findScopedHidden(input, hiddenName);
	if (hidden) hidden.value = "";
	showDropdownOptions(input);
	maybeAutoFillQuantity(input);
});

// "+ Add" affordance inside a dosage/duration dropdown (zero client-side
// matches): POST category+value, get back the refreshed picklist as JSON
// (these are cached client-side, not server-rendered), sync every field
// sharing the same options cache, then auto-apply the new entry.
document.addEventListener("click", function (e) {
	var btn = e.target.closest("[data-quick-add-lookup]");
	if (!btn || !lastCatalogSearchInput) return;

	var input = lastCatalogSearchInput;
	var endpoint = input.getAttribute("data-quick-add-endpoint");
	var category = input.getAttribute("data-lookup-category");
	var value = input.value.trim();
	if (!endpoint || !category || !value) return;

	btn.disabled = true;
	var body = new URLSearchParams();
	body.set("category", category);
	body.set("value", value);

	fetch(endpoint, {
		method: "POST",
		headers: { "Content-Type": "application/x-www-form-urlencoded" },
		body: body.toString(),
	})
		.then(function (resp) {
			if (!resp.ok) throw new Error("quick-add failed: " + resp.status);
			return resp.json();
		})
		.then(function (options) {
			var optionsID = input.getAttribute("data-options-id");
			var src = optionsID && document.getElementById(optionsID);
			if (src) src.textContent = JSON.stringify(options);
			document.querySelectorAll('[data-options-id="' + optionsID + '"]').forEach(function (el) {
				el._options = options;
			});

			var match = options.filter(function (o) {
				return o.value.toLowerCase() === value.toLowerCase();
			})[0];
			input.value = match ? match.value : value;
			var hiddenName = input.getAttribute("data-hidden-id");
			var hidden = hiddenName && findScopedHidden(input, hiddenName);
			if (hidden && match) hidden.value = match.id;

			hideDropdown(document.getElementById(input.getAttribute("data-results")));
			lastCatalogSearchInput = null;
			maybeAutoFillQuantity(input);
		})
		.catch(function (err) {
			console.error(err);
			alert("Couldn't add that. Please try again.");
		})
		.finally(function () {
			btn.disabled = false;
		});
});

// Patient search-or-create widget (used by both the staff booking screen
// and Quick Prescription): one search box, pick a match, or — auto-shown
// when a search finds nothing, or via the manual override button — an
// inline "new patient" panel. Scoped to the nearest [data-patient-widget]
// rather than hardcoded element ids, so more than one instance can exist.
function showPatientNewPanel(widget, prefillQuery) {
	var panel = widget.querySelector("[data-patient-new-panel]");
	if (!panel) return;
	panel.hidden = false;
	panel.querySelectorAll("input, select, textarea").forEach(function (f) { f.disabled = false; });
	if (!prefillQuery) return;
	var trimmed = prefillQuery.trim();
	var phone = panel.querySelector('[name="phone"]');
	if (/^[\d\s+()-]+$/.test(trimmed) && phone && !phone.value) {
		phone.value = trimmed;
		return;
	}
	var first = panel.querySelector('[name="first_name"]');
	var last = panel.querySelector('[name="last_name"]');
	var parts = trimmed.split(/\s+/);
	if (first && !first.value) first.value = parts[0] || "";
	if (last && !last.value) last.value = parts.slice(1).join(" ");
}

function hidePatientNewPanel(widget) {
	var panel = widget.querySelector("[data-patient-new-panel]");
	if (!panel) return;
	panel.hidden = true;
	panel.querySelectorAll("input, select, textarea").forEach(function (f) { f.disabled = true; });
}

document.addEventListener("input", function (e) {
	var input = e.target.closest("[data-patient-search]");
	if (!input) return;
	var widget = input.closest("[data-patient-widget]");
	if (!widget) return;
	var results = widget.querySelector("[data-patient-results]");
	if (!results) return;

	var label = widget.querySelector("[data-patient-selected-label]");
	if (label) label.hidden = true;
	var hidden = widget.querySelector('input[name="patient_id"]');
	if (hidden) hidden.value = "";

	clearTimeout(input._searchTimer);
	var value = input.value.trim();
	if (!value) {
		results.innerHTML = "";
		hidePatientNewPanel(widget);
		return;
	}

	var endpoint = widget.getAttribute("data-search-endpoint");
	input._searchTimer = setTimeout(function () {
		fetch(endpoint + "?q=" + encodeURIComponent(value))
			.then(function (resp) { return resp.ok ? resp.text() : ""; })
			.then(function (html) {
				results.innerHTML = html;
				var inner = results.querySelector("[data-patient-results-inner]");
				var count = inner ? parseInt(inner.getAttribute("data-count"), 10) || 0 : 0;
				if (count === 0) {
					showPatientNewPanel(widget, value);
				} else {
					hidePatientNewPanel(widget);
				}
			})
			.catch(function (err) { console.error(err); });
	}, 300);
});

document.addEventListener("click", function (e) {
	var row = e.target.closest("[data-select-patient]");
	if (!row) return;
	var widget = row.closest("[data-patient-widget]");
	if (!widget) return;

	var id = row.getAttribute("data-id");
	var name = row.getAttribute("data-name");
	var navigateTemplate = widget.getAttribute("data-navigate-template");
	if (navigateTemplate) {
		location.href = navigateTemplate.replace("{id}", id);
		return;
	}

	var hidden = widget.querySelector('input[name="patient_id"]');
	if (hidden) hidden.value = id;
	var search = widget.querySelector("[data-patient-search]");
	if (search) search.value = name;
	var results = widget.querySelector("[data-patient-results]");
	if (results) results.innerHTML = "";

	var label = widget.querySelector("[data-patient-selected-label]");
	if (label) {
		label.textContent = "";
		label.appendChild(document.createTextNode(name + " selected — "));
		var change = document.createElement("a");
		change.href = "#";
		change.textContent = "change";
		change.setAttribute("data-patient-change", "");
		label.appendChild(change);
		label.hidden = false;
	}
	hidePatientNewPanel(widget);
});

document.addEventListener("click", function (e) {
	var link = e.target.closest("[data-patient-change]");
	if (!link) return;
	e.preventDefault();
	var widget = link.closest("[data-patient-widget]");
	if (!widget) return;
	var hidden = widget.querySelector('input[name="patient_id"]');
	if (hidden) hidden.value = "";
	var label = widget.querySelector("[data-patient-selected-label]");
	if (label) label.hidden = true;
	var search = widget.querySelector("[data-patient-search]");
	if (search) {
		search.value = "";
		search.focus();
	}
});

document.addEventListener("click", function (e) {
	var btn = e.target.closest("[data-patient-add-new-toggle]");
	if (!btn) return;
	var widget = btn.closest("[data-patient-widget]");
	if (!widget) return;
	var panel = widget.querySelector("[data-patient-new-panel]");
	if (panel && !panel.hidden) {
		hidePatientNewPanel(widget);
	} else {
		showPatientNewPanel(widget, null);
	}
});

// Account page tabs: show-one-panel-at-a-time, driven by the URL hash so
// a redirect back from a form submit lands on the right tab.
function activateAccountTab(nav, targetID) {
	nav.querySelectorAll("[data-tab-toggle]").forEach(function (btn) {
		var isTarget = btn.getAttribute("data-tab-target") === targetID;
		btn.classList.toggle("is-active", isTarget);
		var panel = document.getElementById(btn.getAttribute("data-tab-target"));
		if (panel) panel.hidden = !isTarget;
	});
}

document.addEventListener("click", function (e) {
	var btn = e.target.closest("[data-tab-toggle]");
	if (!btn) return;
	var nav = btn.closest("[data-tabs]");
	if (!nav) return;
	var targetID = btn.getAttribute("data-tab-target");
	activateAccountTab(nav, targetID);
	if (history.replaceState) history.replaceState(null, "", "#" + targetID.replace(/^tab-/, ""));
});

document.querySelectorAll("[data-tabs]").forEach(function (nav) {
	var targetID = location.hash ? "tab-" + location.hash.slice(1) : null;
	if (!targetID || !nav.querySelector('[data-tab-target="' + targetID + '"]')) {
		var firstBtn = nav.querySelector("[data-tab-toggle]");
		targetID = firstBtn && firstBtn.getAttribute("data-tab-target");
	}
	if (targetID) activateAccountTab(nav, targetID);
});
