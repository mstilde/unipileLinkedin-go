// Minimal client-side helpers for the sequence builder. The "save" still goes
// through HTMX → /ui/.../sequence; this file only handles add/remove + the
// form-to-JSON conversion used by the PUT handler.
(function () {
  const stack = document.getElementById("step-stack");
  if (!stack) return;

  window.appendStep = function (stepType) {
    const idx = stack.children.length;
    const card = document.createElement("details");
    card.className = "step-card";
    card.open = true;
    card.dataset.stepIndex = String(idx);
    card.innerHTML = `
      <summary>
        <span class="badge">#${idx + 1}</span>
        <span class="step-type">${stepType}</span>
      </summary>
      <div class="step-body">
        <label>Type
          <select name="steps[${idx}].step_type">
            ${["invite","message","wait","visit_profile","voice_note","inmail","end"].map(t =>
              `<option value="${t}" ${t===stepType?"selected":""}>${t}</option>`).join("")}
          </select>
        </label>
        <label>Delay (hours)
          <input type="number" name="steps[${idx}].delay_hours" value="${stepType==='wait'?24:0}" min="0"/>
        </label>
        <label class="full">Template
          <textarea name="steps[${idx}].template" rows="4" placeholder="Hola {nombre}, ..."></textarea>
        </label>
        <label>Char limit
          <input type="number" name="steps[${idx}].note_max_chars" value="200" min="1" max="2000"/>
        </label>
        <label>Stage label
          <input type="text" name="steps[${idx}].stage_label" placeholder="optional"/>
        </label>
        <button type="button" class="danger" onclick="removeStep(this)">Remove step</button>
      </div>`;
    stack.appendChild(card);
  };

  window.removeStep = function (btn) {
    const card = btn.closest(".step-card");
    if (card) card.remove();
    // Re-number cards
    [...stack.children].forEach((c, i) => {
      c.dataset.stepIndex = String(i);
      const badge = c.querySelector(".badge");
      if (badge) badge.textContent = "#" + (i + 1);
      c.querySelectorAll("[name^='steps[']").forEach(el => {
        el.name = el.name.replace(/^steps\[\d+\]/, `steps[${i}]`);
      });
    });
  };

  // Convert the form's flat field names into the JSON body expected by /ui/...sequence (PUT).
  const form = document.getElementById("sequence-form");
  if (form) {
    form.addEventListener("htmx:configRequest", (e) => {
      const data = new FormData(form);
      const buckets = {};
      for (const [name, value] of data.entries()) {
        const m = name.match(/^steps\[(\d+)\]\.(.+)$/);
        if (!m) continue;
        const i = +m[1], field = m[2];
        buckets[i] = buckets[i] || {};
        if (field === "delay_hours" || field === "note_max_chars") {
          buckets[i][field] = +value;
        } else if (value !== "") {
          buckets[i][field] = value;
        }
      }
      const steps = Object.keys(buckets).sort((a, b) => +a - +b).map(k => buckets[k]);
      e.detail.parameters = {};
      e.detail.headers["Content-Type"] = "application/json";
      e.detail.body = JSON.stringify({ steps });
    });
  }
})();
