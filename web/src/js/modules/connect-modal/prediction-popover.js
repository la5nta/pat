import $ from 'jquery';

class PredictionPopover {
  constructor() {
    this.activePopovers = [];
  }

  /**
   * Initialize a popover on an element with prediction output values
   * @param {jQuery} element - The element to attach the popover to
   * @param {Object} outputValues - The prediction output values to display
   * @param {string} title - The title for the popover
   * @return {jQuery} - The element with popover attached
   */
  attach(element, outputValues, title = 'Prediction Details') {
    if (!element || !outputValues) {
      return element;
    }

    // Format popover content with one KV-pair per line
    let popoverContent = '';
    Object.entries(outputValues).forEach(([key, value]) => {
      popoverContent += `${key}: ${value}\n`;
    });

    // Only create popover if we have content
    if (popoverContent) {
      element
        .attr('data-toggle', 'popover')
        .attr('data-html', 'true')
        .attr('data-content', '<pre style="text-align: left; margin: 0; background: transparent; border: 0; padding: 0;">' + popoverContent.trim() + '</pre>')
        .attr('data-placement', 'left')
        .attr('title', title);

      // Initialize popover
      element.popover({
        container: 'body',
        html: true,
        trigger: 'hover',
        template: '<div class="popover" role="tooltip"><div class="arrow"></div><h3 class="popover-title"></h3><div class="popover-content"></div></div>'
      });

      // Track this popover so we can clean it up later
      this.activePopovers.push(element);
    }

    return element;
  }

  /**
   * Hide a specific popover
   * @param {jQuery} element - The element with the popover
   */
  hide(element) {
    if (element && element.data('bs.popover')) {
      element.popover('hide');
    }
  }

  /**
   * Destroy all active popovers
   */
  destroyAll() {
    $('.link-quality-cell span[data-toggle="popover"]').popover('destroy');
    this.activePopovers = [];
  }
}

export { PredictionPopover };
