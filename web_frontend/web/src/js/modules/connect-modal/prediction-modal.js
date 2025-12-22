import $ from 'jquery';

class PredictionModal {
  constructor() {
    this.modalId = 'rawDataModal';
  }

  /**
   * Show a modal with the raw prediction output
   * @param {string} callsign - The station callsign
   * @param {string} rawOutput - The raw prediction output text
   */
  show(callsign, rawOutput) {
    if (!rawOutput) {
      return;
    }

    // Remove any existing modal
    this.remove();

    // Create modal with scrollable content
    let modalHtml =
      `<div class="modal fade" id="${this.modalId}" tabindex="-1" role="dialog">
        <div class="modal-dialog modal-lg">
          <div class="modal-content">
            <div class="modal-header">
              <button type="button" class="close" data-dismiss="modal" aria-label="Close">
                <span aria-hidden="true">&times;</span>
              </button>
              <h4 class="modal-title">Propagation Prediction Details: ${callsign}</h4>
            </div>
            <div class="modal-body">
              <div style="overflow: auto;">
                <pre style="white-space: pre; width: auto; margin-bottom: 0;">${rawOutput}</pre>
              </div>
            </div>
            <div class="modal-footer">
              <button type="button" class="btn btn-default" data-dismiss="modal">Close</button>
            </div>
          </div>
        </div>
      </div>`;

    // Add the modal to the body
    $('body').append(modalHtml);

    // Show the modal
    $(`#${this.modalId}`).modal('show');
  }

  /**
   * Remove the modal from the DOM
   */
  remove() {
    $(`#${this.modalId}`).remove();
  }
}

export { PredictionModal };
