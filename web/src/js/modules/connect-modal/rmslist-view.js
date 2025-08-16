import $ from 'jquery';
import { PredictionPopover } from './prediction-popover';
import { PredictionModal } from './prediction-modal';

class RmslistView {
  constructor() {
    this.predictionPopover = new PredictionPopover();
    this.predictionModal = new PredictionModal();
    this.rmslistData = [];
    this.filteredData = [];
    this.itemsShown = 0;
    this.itemsPerLoad = 100;
    this.hideLinkQuality = true;
    this.onRowClick = null;
  }

  init() {
    $('#targetFilterInput').on('input', () => this.filterRmslist());
    $('#loadmore-btn').click(() => {
      this.loadMoreItems();
    });

    $('#updateRmslistButton').click((e) => {
      $(e.target).prop('disabled', true);
      this.updateRmslist(true);
    });

    $('#modeSearchSelect').change(() => {
      this.updateRmslist();
    });

    $('#bandSearchSelect').change(() => {
      this.updateRmslist();
    });

    $('button[data-target="#rmslist-container"]').click(() => {
      this.updateRmslist();
    });
  }

  onTransportChange(transport) {
    // Update rmslist view based on transport
    switch (transport) {
      case 'ardop':
      case 'pactor':
      case 'varafm':
      case 'varahf':
        $('#modeSearchSelect').val(transport);
        break;
      case 'ax25':
      case 'ax25+linux':
      case 'ax25+agwpe':
      case 'ax25+serial-tnc':
        $('#modeSearchSelect').val('packet');
        break;
      default:
        return;
    }
    $('#modeSearchSelect').selectpicker('refresh');

    // Refresh the RMS list with the new filter
    this.updateRmslist();
  }

  updateRmslist(forceDownload) {
    let tbody = $('#rmslist tbody');

    this.predictionModal.remove();
    this.predictionPopover.destroyAll();

    let params = {
      mode: $('#modeSearchSelect').val(),
      band: $('#bandSearchSelect').val(),
      'force-download': forceDownload === true,
    };

    $.ajax({
      method: 'GET',
      url: '/api/rmslist',
      dataType: 'json',
      data: params,
      beforeSend: () => {
        tbody.empty();
        $('#rmslistSpinner').show();
        $('#rmslist-loadmore').hide();
      },
      success: (data) => {
        this.rmslistData = data;
        this.itemsShown = 0;
        this.filterRmslist();
      },
      complete: () => {
        $('#rmslistSpinner').hide();
        $('#updateRmslistButton').prop('disabled', false);
      },
    });
  }

  filterRmslist() {
    const filterText = $('#targetFilterInput').val().toLowerCase();
    this.filteredData = this.rmslistData.filter(rms =>
      rms.callsign.toLowerCase().startsWith(filterText)
    );
    this.itemsShown = Math.min(this.itemsPerLoad, this.filteredData.length);
    this.hideLinkQuality = this.filteredData.every(rms => rms.prediction == null);

    const initialData = this.filteredData.slice(0, this.itemsShown);
    this.renderRmslist(initialData, this.hideLinkQuality);
    this.updateLoadMoreControls();
  }

  loadMoreItems() {
    const currentItems = this.itemsShown;
    this.itemsShown = Math.min(this.itemsShown + this.itemsPerLoad, this.filteredData.length);
    const newItems = this.filteredData.slice(currentItems, this.itemsShown);
    this.appendToRmslist(newItems);
    this.updateLoadMoreControls();
  }

  updateLoadMoreControls() {
    $('#showing-count').text(this.itemsShown);
    $('#total-results').text(this.filteredData.length);

    const hasMore = this.itemsShown < this.filteredData.length;
    $('#loadmore-btn').toggle(hasMore);
    $('#rmslist-loadmore').toggle(this.filteredData.length > 0);
  }

  appendToRmslist(data) {
    this.renderRmslistRows(data, this.hideLinkQuality);
  }

  renderRmslist(data, hideLinkQuality) {
    let tbody = $('#rmslist tbody');
    tbody.empty();
    $('.link-quality-column').toggle(!hideLinkQuality);
    this.renderRmslistRows(data, hideLinkQuality);
  }

  renderRmslistRows(data, hideLinkQuality) {
    let tbody = $('#rmslist tbody');

    data.forEach((rms) => {
      let tr = $('<tr>')
        .append($('<td class="text-left">').text(rms.callsign))
        .append($('<td class="text-left">').text(rms.distance.toFixed(0) + ' km'))
        .append($('<td class="text-left">').text(rms.modes))
        .append($('<td class="text-right">').text(rms.dial.desc));

      let linkQualityCell = $('<td class="text-right link-quality-cell">');
      if (hideLinkQuality) {
        linkQualityCell.hide();
      } else {
        let linkQualityText = rms.prediction == null ? 'N/A' : rms.prediction.link_quality + '%';
        let span = $('<span>').text(linkQualityText);
        if (rms.prediction) {
          span.css('cursor', 'pointer').css('border-bottom', '1px dotted #337ab7');
          if (rms.prediction.output_values) {
            this.predictionPopover.attach(span, rms.prediction.output_values);
          }
          if (rms.prediction.output_raw) {
            span.on('click', (e) => {
              e.stopPropagation();
              e.preventDefault();
              this.predictionPopover.hide(span);
              this.predictionModal.show(rms.callsign, rms.prediction.output_raw);
              return false;
            });
          }
        }
        linkQualityCell.append(span);
      }
      tr.append(linkQualityCell);

      tr.click((e) => {
        tbody.find('.active').removeClass('active');
        tr.addClass('active');
        if (this.onRowClick) {
          this.onRowClick(rms.url);
        }
      });
      tbody.append(tr);
    });
  }
}

export { RmslistView };
