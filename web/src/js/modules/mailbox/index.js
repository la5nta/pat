import $ from 'jquery';
import { htmlEscape } from '../utils/index.js';

export class Mailbox {
  constructor(onMessageClick) {
    this.onMessageClick = onMessageClick;
    this.currentFolder = 'in';
    this.selected = new Set();
    this.bulkBusy = false;
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
      if (evt.currentTarget.checked) {
        this.selected.add(mid);
      } else {
        this.selected.delete(mid);
      }
      this._updateBulkBar();
    });
    this.table.on('change', 'th.select-col input', (evt) => {
      const checked = evt.currentTarget.checked;
      this.table.find('td.select-col input').each((_, box) => {
        box.checked = checked;
        const mid = $(box).closest('tr').attr('id');
        if (checked) {
          this.selected.add(mid);
        } else {
          this.selected.delete(mid);
        }
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
      // A bulk operation refreshes the folder itself when it is done
      return;
    }
    if (dir !== this.currentFolder) {
      this.selected.clear();
    }
    this.currentFolder = dir;
    const is_from = dir === 'in' || dir === 'archive';

    this.table.empty();
    this.table.append(
      `<thead><tr><th class="select-col"><input type="checkbox" title="Select all" /></th>
      <th></th><th class="sortable">Subject</th>
      <th class="sortable">${is_from ? 'From' : 'To'}</th>
      ${is_from ? '' : '<th class="sortable">P2P</th>'}
      <th class="sortable">Date</th><th>Message ID</th></tr></thead><tbody></tbody>`
    );

    const tbody = this.table.find('tbody');
    this._updateBulkBar();

    $.getJSON(`/api/mailbox/${dir}`, (data) => {
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
    });
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

    const idx = ths.indexOf(th);
    const midIdx = ths.length - 1;
    const isDate = th.textContent.trim() === 'Date';
    const asc = this.sort.asc;
    const value = (tr, i) => this._getCellValue(tr, i);
    const compare = (a, b) => {
      let v1 = value(a, idx);
      let v2 = value(b, idx);
      let res;
      if (isDate) {
        res = (Date.parse(v1) || 0) - (Date.parse(v2) || 0);
      } else if (v1 !== '' && v2 !== '' && !isNaN(v1) && !isNaN(v2)) {
        res = v1 - v2;
      } else {
        res = v1.toString().localeCompare(v2, undefined, { sensitivity: 'base' });
      }
      if (res === 0) {
        // Message ID breaks ties so equal rows keep a stable order
        res = value(a, midIdx).localeCompare(value(b, midIdx));
      }
      return asc ? res : -res;
    };

    const tbody = this.table.find('tbody')[0];
    Array.from(tbody.querySelectorAll('tr'))
      .sort(compare)
      .forEach((tr) => tbody.appendChild(tr));

    ths.forEach((h) => h.classList.remove('sorted', 'sorted-asc', 'sorted-desc'));
    th.classList.add('sorted', asc ? 'sorted-asc' : 'sorted-desc');
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

  _messagePath(mid) {
    return '/api/mailbox/' + encodeURIComponent(this.currentFolder) + '/' + encodeURIComponent(mid);
  }

  // Runs request(mid) for every selected message, one at a time, then
  // refreshes the folder and reports any failures. The selection is kept so
  // an action can be reversed right away (e.g. mark unread after mark read).
  // Messages that left the folder are dropped from it by displayFolder.
  async _bulkRun(request) {
    if (this.bulkBusy) {
      return;
    }
    this.bulkBusy = true;
    this.bulkBar.find('button').prop('disabled', true);

    const mids = Array.from(this.selected);
    const failed = [];
    for (let i = 0; i < mids.length; i++) {
      this.bulkCount.text(`${i + 1} of ${mids.length}...`);
      try {
        await request(mids[i]);
      } catch (xhr) {
        failed.push(`${mids[i]}: ${xhr.responseText || xhr.statusText}`);
      }
    }

    this.bulkBar.find('button').prop('disabled', false);
    this.bulkBusy = false;
    this.displayFolder(this.currentFolder);
    if (failed.length > 0) {
      alert(`${failed.length} of ${mids.length} failed:\n` + failed.join('\n'));
    }
  }

  _bulkSetRead(read) {
    this._bulkRun((mid) =>
      $.ajax(this._messagePath(mid) + '/read', {
        data: JSON.stringify({ read: read }),
        contentType: 'application/json',
        type: 'POST',
      })
    );
  }

  _bulkMove(target) {
    this._bulkRun((mid) =>
      $.ajax('/api/mailbox/' + encodeURIComponent(target), {
        headers: { 'X-Pat-SourcePath': this._messagePath(mid) },
        contentType: 'application/json',
        type: 'POST',
      })
    );
  }

  _bulkDelete() {
    if (!confirm(`Delete ${this.selected.size} message(s)? This cannot be undone.`)) {
      return;
    }
    this._bulkRun((mid) => $.ajax(this._messagePath(mid), { type: 'DELETE' }));
  }

  _getCellValue(tr, idx) {
    return tr.children[idx].innerText || tr.children[idx].textContent;
  }
}
