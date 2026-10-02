import $ from 'jquery';
import { alert, htmlEscape } from '../utils/index.js';
import { setRead, moveMessage, deleteMessage } from '../message-api/index.js';

// How many bulk requests are in flight at once
const BULK_CONCURRENCY = 4;
// How many per-message failures the summary lists before "and N more"
const MAX_FAILURES_SHOWN = 10;

export class Mailbox {
  constructor(onMessageClick) {
    this.onMessageClick = onMessageClick;
    this.currentFolder = 'in';
    this.selected = new Set();
    this.bulkBusy = false;
    // A folder requested during a bulk run, shown once the run is done
    this.pendingFolder = null;
    // Counts folder loads so a late response from an older load is ignored
    this.loadSeq = 0;
    // Sort state survives folder refreshes. Newest first by default.
    this.sort = { label: 'Date', asc: false };
  }

  init() {
    this.folder = $('#folder');
    this.table = this.folder.find('table');
    this.bulkBar = $('#bulk_actions');
    this.bulkCount = $('#bulk_count');

    // Adapted from https://stackoverflow.com/a/49041392
    this.table.on('click', 'th.sortable', (event) => {
      const label = event.currentTarget.textContent.trim();
      this.sort = { label: label, asc: this.sort.label === label ? !this.sort.asc : true };
      this._applySort();
    });

    // Clicking anywhere in the selection cell toggles its checkbox
    this.table.on('click', 'td.select-col', (evt) => {
      if (evt.target.tagName !== 'INPUT') {
        $(evt.currentTarget).find('input').trigger('click');
      }
    });
    this.table.on('change', 'td.select-col input', (evt) => {
      const mid = $(evt.currentTarget).closest('tr').attr('id');
      this._setSelected(mid, evt.currentTarget.checked);
      this._updateBulkBar();
    });
    this.table.on('change', 'th.select-col input', (evt) => {
      const checked = evt.currentTarget.checked;
      this.table.find('td.select-col input').each((_, box) => {
        box.checked = checked;
        this._setSelected($(box).closest('tr').attr('id'), checked);
      });
      this._updateBulkBar();
    });

    $('#bulk_read_btn').click(() => this._bulkSetRead(true));
    $('#bulk_unread_btn').click(() => this._bulkSetRead(false));
    $('#bulk_archive_btn').click(() => this._bulkMove('archive'));
    $('#bulk_unarchive_btn').click(() => this._bulkMove('in'));
    $('#bulk_delete_btn').click(() => this._bulkDelete());
    $('#bulk_clear_btn').click(() => {
      this.selected.clear();
      this.table.find('.select-col input').prop('checked', false);
      this._updateBulkBar();
    });
  }

  displayFolder(dir) {
    if (this.bulkBusy) {
      // The run refreshes the list when it is done; a folder switch waits for
      // it. UpdateMailbox events from the run's own writes arrive here as
      // refreshes of the current folder and must not displace that switch.
      if (dir !== this.currentFolder) {
        this.pendingFolder = dir;
      }
      return;
    }
    if (dir !== this.currentFolder) {
      this.selected.clear();
    }
    this.currentFolder = dir;
    const seq = ++this.loadSeq;
    const is_from = dir === 'in' || dir === 'archive';

    this.table.empty();
    this.table.append(
      `<thead><tr><th class="select-col"><input type="checkbox" title="Select all" /></th>
      <th></th><th class="sortable">Subject</th>
      <th class="sortable">${is_from ? 'From' : 'To'}</th>
      ${is_from ? '' : '<th>P2P</th>'}
      <th class="sortable">Date</th><th>Message ID</th></tr></thead><tbody></tbody>`
    );

    const tbody = this.table.find('tbody');
    this._updateBulkBar();

    $.getJSON(`/api/mailbox/${dir}`)
      .done((data) => {
        if (seq !== this.loadSeq) {
          // A newer load has replaced this one
          return;
        }

        // Drop selections for messages that no longer exist in this folder
        const present = new Set(data.map((msg) => msg.MID));
        this.selected.forEach((mid) => {
          if (!present.has(mid)) {
            this.selected.delete(mid);
          }
        });

        data.forEach((msg) => {
          let to_from_html = '';
          if (!is_from && msg.To) {
            if (msg.To.length === 1) {
              to_from_html = msg.To[0].Addr;
            } else if (msg.To.length > 1) {
              to_from_html = `${msg.To[0].Addr}...`;
            }
          } else if (is_from) {
            to_from_html = msg.From.Addr;
          }

          const p2p_html = is_from
            ? ''
            : `<td>${msg.P2POnly ? '<span class="glyphicon glyphicon-ok"></span>' : ''}</td>`;

          const elem = $(`
            <tr id="${msg.MID}" class="active${msg.Unread ? ' strong' : ''}">
              <td class="select-col"><input type="checkbox"${this.selected.has(msg.MID) ? ' checked' : ''} /></td>
              <td>${msg.Files.length > 0 ? '<span class="glyphicon glyphicon-paperclip"></span>' : ''}</td>
              <td>${htmlEscape(msg.Subject)}</td>
              <td>${to_from_html}</td>
              ${p2p_html}
              <td>${msg.Date}</td>
              <td>${msg.MID}</td>
            </tr>
          `);

          tbody.append(elem);
          elem.click((evt) => {
            // Clicks in the selection column only toggle the checkbox
            if ($(evt.target).closest('.select-col').length > 0) {
              return;
            }

            // Handle active class for the message list
            tbody.find('tr.active').removeClass('active');
            elem.addClass('active');

            this.onMessageClick(this.currentFolder, elem.attr('id'));
          });
        });
        this._applySort();
        this._updateBulkBar();
      })
      .fail((xhr, st, err) => {
        if (seq !== this.loadSeq) {
          return;
        }
        // Nothing is listed, so nothing stays selected
        this.selected.clear();
        this._updateBulkBar();
        alert(`Could not load ${dir}: ${xhr.responseText || err || st}`);
      });
  }

  _setSelected(mid, checked) {
    if (checked) {
      this.selected.add(mid);
    } else {
      this.selected.delete(mid);
    }
  }

  // Orders the rows by the remembered column and marks its header. From and
  // To share a slot, so a sort on one carries over to the other folder type.
  _applySort() {
    const ths = Array.from(this.table[0].querySelectorAll('thead th'));
    const find = (label) =>
      ths.find((th) => th.classList.contains('sortable') && th.textContent.trim() === label);
    let th = find(this.sort.label);
    if (!th && (this.sort.label === 'From' || this.sort.label === 'To')) {
      th = find('From') || find('To');
    }
    if (!th) {
      th = find('Date');
    }
    if (!th) {
      return;
    }
    // Remember the header actually used, so the next click on it toggles
    this.sort.label = th.textContent.trim();

    const idx = ths.indexOf(th);
    const midIdx = ths.length - 1;
    // Date is numeric, every other sortable column is text
    const isDate = th.textContent.trim() === 'Date';
    const text = (tr, i) => tr.children[i].textContent.trim();

    // Each row's key is read once up front; comparing cached keys keeps the
    // sort from touching the DOM on every comparison
    const tbody = this.table.find('tbody')[0];
    const rows = Array.from(tbody.querySelectorAll('tr')).map((tr) => ({
      tr: tr,
      key: isDate ? Date.parse(text(tr, idx)) || 0 : text(tr, idx),
      mid: text(tr, midIdx),
    }));

    const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' });
    const direction = this.sort.asc ? 1 : -1;
    rows.sort((a, b) => {
      const res = isDate ? a.key - b.key : collator.compare(a.key, b.key);
      // Message ID breaks ties so equal rows keep a stable order
      return direction * (res || a.mid.localeCompare(b.mid));
    });
    rows.forEach((row) => tbody.appendChild(row.tr));

    ths.forEach((h) => h.classList.remove('sorted', 'sorted-asc', 'sorted-desc'));
    th.classList.add('sorted', this.sort.asc ? 'sorted-asc' : 'sorted-desc');
  }

  _updateBulkBar() {
    const count = this.selected.size;
    this.bulkCount.text(`${count} selected`);
    this.bulkBar.toggle(count > 0);
    // Archiving from the archive folder makes no sense
    $('#bulk_archive_btn').toggle(this.currentFolder !== 'archive');
    $('#bulk_unarchive_btn').toggle(this.currentFolder === 'archive');

    const boxes = this.table.find('td.select-col input');
    const all = this.table.find('th.select-col input')[0];
    if (all) {
      all.checked = boxes.length > 0 && count === boxes.length;
      all.indeterminate = count > 0 && count < boxes.length;
    }
  }

  // Runs request(mid) for every selected message, a few at a time, then
  // refreshes the folder and reports any failures. The selection is kept so
  // an action can be reversed right away (e.g. mark unread after mark read).
  async _bulkRun(request) {
    if (this.bulkBusy) {
      return;
    }
    this.bulkBusy = true;
    this.bulkBar.find('button').prop('disabled', true);

    const mids = Array.from(this.selected);
    const failed = [];
    let done = 0;
    let next = 0;
    const worker = async () => {
      while (next < mids.length) {
        const mid = mids[next++];
        try {
          await request(mid);
        } catch (err) {
          const xhrText = err && (err.responseText || err.statusText);
          failed.push(`${mid}: ${xhrText || (err instanceof Error ? err.message : String(err))}`);
        }
        this.bulkCount.text(`${++done} of ${mids.length}...`);
      }
    };
    await Promise.all(Array.from({ length: Math.min(BULK_CONCURRENCY, mids.length) }, worker));

    this.bulkBar.find('button').prop('disabled', false);
    this.bulkBusy = false;
    // A folder clicked during the run is shown now; otherwise refresh in place
    const folder = this.pendingFolder || this.currentFolder;
    this.pendingFolder = null;
    this.displayFolder(folder);
    if (failed.length > 0) {
      // A modal rather than the navbar toast: the list is multi-line and
      // should stay up until read. Capped so a wholesale failure stays legible.
      const shown = failed.slice(0, MAX_FAILURES_SHOWN);
      const more = failed.length - shown.length;
      window.alert(
        `${failed.length} of ${mids.length} failed:\n` +
          shown.join('\n') +
          (more > 0 ? `\nand ${more} more` : '')
      );
    }
  }

  _bulkSetRead(read) {
    this._bulkRun((mid) => setRead(this.currentFolder, mid, read));
  }

  _bulkMove(target) {
    this._bulkRun((mid) => moveMessage(this.currentFolder, mid, target));
  }

  _bulkDelete() {
    // The viewer's confirm modal is bound to a single message; a native
    // confirm keeps the bulk path free of modal state.
    if (!window.confirm(`Delete ${this.selected.size} message(s)? This cannot be undone.`)) {
      return;
    }
    this._bulkRun((mid) => deleteMessage(this.currentFolder, mid));
  }
}
