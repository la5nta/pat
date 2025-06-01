import $ from 'jquery';
import 'bootstrap/dist/js/bootstrap';
import _ from 'lodash';

// Template state management
class TemplateNode {
  constructor(name, type, description = '') {
    // Unique identifier for this node in the template
    this.name = name;
    // Type of node: 'var', 'ask', or 'select'
    this.type = type;
    // Human-readable description of what this node represents
    this.description = description;
    // Current value of this node (user input or computed)
    this.value = '';
    // Set of node names that this node depends on
    this.parents = new Set();
    // Set of node names that depend on this node
    this.children = new Set();
    // Array of {label, value} pairs for select nodes
    this.options = [];
  }
}

class TemplateState {
  constructor() {
    this.nodes = new Map();
    this.domElements = new Map();
    this.templateName = '';
    this.prompts = new Map(); // Store original Ask/Select tags by variable name
  }

  // Get stored prompt for a variable
  getPrompt(name) {
    return this.prompts.get(name);
  }

  // Store a prompt definition
  setPrompt(name, originalTag) {
    this.prompts.set(name, originalTag);
  }

  // Collect all form values into submission format
  collectFormData() {
    const formData = {
      responses: {}
    };

    // Find all input elements from Ask and Select tags
    $('input[data-original-tag], textarea[data-original-tag], select[data-original-tag]').each(function() {
      const $input = $(this);
      const originalTag = $input.data('original-tag');
      const value = $input.val();

      if (originalTag && value) {
        formData.responses[originalTag] = value;
      }
    });

    return formData;
  }

  // Get the latest value from DOM for a node
  getLatestValue(nodeName) {
    const varInput = $(`input[data-var="${nodeName}"]`).first();
    if (varInput.length) {
      return varInput.val();
    }

    const nodeInput = $(`[data-node="${nodeName}"]`).first();
    if (nodeInput.length) {
      return nodeInput.val();
    }

    const node = this.getNode(nodeName);
    return node ? node.value : '';
  }

  addNode(node) {
    this.nodes.set(node.name, node);
  }

  getNode(name) {
    return this.nodes.get(name);
  }

  updateValue(name, value) {
    const node = this.getNode(name);
    if (node) {
      node.value = value;
      this.updateDependents(node);
    }
  }

  updateDependents(node) {
    node.children.forEach(childName => {
      const child = this.getNode(childName);
      if (child) {
        // Update DOM elements that depend on this value
        const elements = this.domElements.get(childName);
        if (elements) {
          elements.forEach(el => {
            $(el).text(child.value);
          });
        }
      }
    });
  }
}

// Command to label mapping with variants
const COMMAND_LABELS = {
  'Type': 'Message Type',
  'To': 'To',
  'CC': 'CC',
  'Cc': 'CC',
  'Subj': 'Subject',
  'Subject': 'Subject',
  'Attach': 'Attachments',
  'SeqSet': 'Sequence Number',
  'SeqInc': 'Increment Sequence',
  'Def': 'Define Variable',
  'Define': 'Define Variable',
  'Readonly': 'Read Only',
  'Form': 'Form Names',
  'ReplyTemplate': 'Reply Template',
  'Msg': 'Message Body'
};

// Template parsing
function parseTemplate(content) {
  const state = new TemplateState();
  const lines = content.split('\n');

  // First pass: collect variable definitions and prompts
  lines.forEach(line => {
    if (line.toLowerCase().startsWith('def:')) {
      const def = parseDef(line);
      if (def) {
        state.addNode(new TemplateNode(def.name, 'var', def.description));
        // Extract and store the Ask/Select tag from the definition
        const promptMatch = line.match(/<(Ask|Select)[^>]+>/);
        if (promptMatch) {
          state.setPrompt(def.name, promptMatch[0]);
        }
      }
    }
  });

  // Second pass: find variable references and build dependency graph
  const varRegex = /<Var\s+([^>]+)>/gi;
  const askRegex = /<Ask\s+([^>]+)>/gi;
  const selectRegex = /<Select\s+([^:]+):([^>]+)>/gi;

  lines.forEach(line => {
    // Find Var references
    let match;
    while ((match = varRegex.exec(line)) !== null) {
      const varName = match[1];
      if (!state.getNode(varName)) {
        state.addNode(new TemplateNode(varName, 'var'));
      }
    }

    // Find Ask prompts
    while ((match = askRegex.exec(line)) !== null) {
      const [prompt, options] = parseAskPrompt(match[1]);
      const node = new TemplateNode(prompt, 'ask');
      if (options) {
        node.options = options;
      }
      state.addNode(node);
    }

    // Find Select options
    while ((match = selectRegex.exec(line)) !== null) {
      const [name, options] = parseSelectOptions(match[1], match[2]);
      const node = new TemplateNode(name, 'select');
      node.options = options;
      state.addNode(node);
    }
  });

  return state;
}

function parseDef(line) {
  const match = /Def:\s*([^=]+)=<([^>]+)>/i.exec(line);
  if (match) {
    return {
      name: match[1].trim(),
      description: match[2].trim()
    };
  }
  return null;
}

function parseAskPrompt(text) {
  const parts = text.split(',');
  const prompt = parts[0].trim();
  const options = parts.slice(1);
  return [prompt, options];
}

function extractDescription(text) {
  const descMatch = text.match(/\((.*?)\)/);
  return descMatch ? descMatch[1] : null;
}

function renderPrompt(originalTag, name, id, extraAttrs = {}) {
  // If it's a Select prompt
  if (originalTag.startsWith('<Select')) {
    const selectMatch = originalTag.match(/<Select\s+([^:]+):([^>]+)>/);
    if (selectMatch) {
      const [, selectName, options] = selectMatch;
      const parsedOptions = parseSelectOptions(selectName, options)[1];
      return renderSelect(name, id, originalTag, parsedOptions, extraAttrs);
    }
  }

  // Parse Ask prompt options
  const isMultiline = originalTag.includes(',MU');
  const isUppercase = originalTag.includes(',UP') || originalTag.includes(',UPPERCASE');
  const description = extractDescription(originalTag);

  if (isMultiline) {
    return renderTextarea(name, id, originalTag, description, isUppercase, extraAttrs);
  }

  return renderInput(name, id, originalTag, description, isUppercase, extraAttrs);
}

function renderSelect(name, id, originalTag, options, extraAttrs = {}) {
  const attrs = Object.entries(extraAttrs)
    .map(([key, value]) => `${key}="${value}"`)
    .join(' ');

  const optionsHtml = options
    .map(opt => `<option value="${opt.value}">${opt.label}</option>`)
    .join('');

  return `<select class="form-select template-select"
            data-node="${name}"
            id="${id}"
            data-original-tag="${originalTag}"
            ${attrs}>
            <option value="">Choose ${name}</option>
            ${optionsHtml}
            </select>`;
}

function renderTextarea(name, id, originalTag, description, isUppercase, extraAttrs = {}) {
  const attrs = Object.entries(extraAttrs)
    .map(([key, value]) => `${key}="${value}"`)
    .join(' ');

  return `<textarea class="form-control p-1"
                  data-node="${name}"
                  id="${id}"
                  title="${description || name}"
                  placeholder="${description || name}"
                  data-original-tag="${originalTag}"
                  ${isUppercase ? 'data-uppercase="true"' : ''}
                  ${attrs}></textarea>`;
}

function renderInput(name, id, originalTag, description, isUppercase, extraAttrs = {}) {
  const attrs = Object.entries(extraAttrs)
    .map(([key, value]) => `${key}="${value}"`)
    .join(' ');

  return `<input type="text"
          class="var-input"
          data-node="${name}"
          id="${id}"
          title="${description || name}"
          placeholder="${description || name}"
          data-original-tag="${originalTag}"
          ${isUppercase ? 'data-uppercase="true"' : ''}
          // Prevent password managers and form autofill from triggering
          autocomplete="off"
          data-form-type="other"
          // Explicitly tell 1Password and lastpass to ignore these fields
          data-lpignore="true" data-1p-ignore="true"
          ${attrs}>`;
}

function parseSelectOptions(name, optionsStr) {
  const options = optionsStr.split(',').map(opt => {
    const [label, value] = opt.split('=');
    return { label: label.trim(), value: (value ? value.trim() : label.trim()) };
  });
  return [name.trim(), options];
}

// Convert template to interactive HTML
async function templateToHtml(content, state) {
  // Split content into headers and message
  content = content.replace(/\r\n/g, '\n');
  const parts = content.split(/Msg:\s*\n/i);
  const headerSection = parts[0];
  const messageSection = parts.length > 1 ? parts[1] : '';

  // Split headers section into definitions and headers
  const headerLines = headerSection.split('\n');
  const definitions = [];
  const headers = [];

  // Separate Def: lines from other headers
  headerLines.forEach(line => {
    if (line.trim().startsWith('Def:')) {
      definitions.push(line);
    } else {
      headers.push(line);
    }
  });

  // Process headers section
  // First, separate To and Subject headers
  const priorityHeaders = [];
  const otherHeaders = [];

  headers.forEach(line => {
    if (line.startsWith('To:') || line.startsWith('Subj:')) {
      priorityHeaders.push(line);
    } else {
      otherHeaders.push(line);
    }
  });

  // Combine headers with priority headers first
  const orderedHeaders = [...priorityHeaders, ...otherHeaders];
  let headerHtml = await processSection(orderedHeaders.join('\n'), state, true);
  $('#headers_list').html(headerHtml);

  // Process message section
  let messageHtml = await processSection(messageSection, state, false);
  $('#message_content').html(messageHtml);

  return ''; // No longer need to return HTML
}

// Helper function to process template sections
async function processSection(content, state, isHeader = false) {
  // Process lines with command labels
  let html = content.split('\n').map(line => {
    // Skip empty lines
    if (!line.trim()) return line;

    // Handle command lines
    const colonIndex = line.indexOf(':');
    if (colonIndex > 0) {
      const command = line.substring(0, colonIndex).trim();
      const rest = line.substring(colonIndex + 1);

      // If it's a header section and we have a label for this command
      if (isHeader && COMMAND_LABELS[command]) {
        // For commands with variants (e.g. Subject/Subj), use the canonical label
        return `${COMMAND_LABELS[command]}: ${rest}`;
      }

      // Special handling for Def: lines
      if (line.startsWith('Def:')) {
        const def = parseDef(line);
        if (def && state.defLines.has(def.name)) {
          // Extract just the variable name and description
          const node = state.getNode(def.name);
          if (node) {
            return `${def.name.trim()}: <${node.description}>`;
          }
        }
      }
    }
    return line;
  }).join('\n');

  // Replace <Ask> tags with input fields that stay inline
  html = html.replace(/<Ask\s+([^>]+)>/gi, (match, content) => {
    const [prompt] = parseAskPrompt(content);
    const id = _.uniqueId('ask_');
    const node = state.getNode(prompt) || new TemplateNode(prompt, 'ask');
    state.addNode(node);

    return renderPrompt(match, prompt, id);
  });

  // Replace <Select> tags with dropdown menus
  html = html.replace(/<Select\s+([^:]+):([^>]+)>/gi, (match, name, options) => {
    const id = _.uniqueId('select_');
    const node = state.getNode(name) || new TemplateNode(name, 'select');
    node.options = parseSelectOptions(name, options)[1];
    state.addNode(node);

    const optionsHtml = node.options
      .map(opt => `<option value="${opt.value}">${opt.label}</option>`)
      .join('');

    return `<select class="form-select template-select"
                data-node="${name}"
                id="${id}"
                data-original-tag="${match}">
                <option value="">Choose ${name}</option>
                ${optionsHtml}
                </select>`;
  });

  // Replace <Var> references with input fields
  html = html.replace(/<Var\s+([^>]+)>/gi, (match, name) => {
    const id = _.uniqueId('var_');
    const node = state.getNode(name) || new TemplateNode(name, 'var');
    state.addNode(node);

    // Get the stored prompt for this variable
    const originalTag = state.getPrompt(name) || match;

    return renderPrompt(originalTag, name, id, {
      'data-var': name,
      'value': node.value || ''
    });
  });

  return html;
}

// Handle variable input field focus and changes
function setupVariableHandlers() {
  $('.container').on('focus', 'input[data-var]', function() {
    const varName = $(this).data('var');
    $(`input[data-var="${varName}"]`).addClass('linked-active');
  });

  $('.container').on('blur', 'input[data-var]', function() {
    const varName = $(this).data('var');
    $(`input[data-var="${varName}"]`).removeClass('linked-active');
  });

  $('.container').on('input', 'input[data-var]', function() {
    const varName = $(this).data('var');
    const value = $(this).val();

    // Update all inputs for this variable and resize them
    $(`input[data-var="${varName}"]`).each(function() {
      $(this).val(value);
      updateInputWidth(this);
    });

    // Update state
    state.updateValue(varName, value);
  });
}

// Global state
let state;

// Helper function to check if all prompts are filled
function checkAllPromptsFilled() {
  let allFilled = true;

  // Check all inputs, textareas and selects with data-original-tag
  $('[data-original-tag]').each(function() {
    const value = $(this).val();
    if (!value || value.trim() === '') {
      allFilled = false;
      return false; // break the loop
    }
  });

  // Enable/disable save button based on result
  $('#save').prop('disabled', !allFilled);
}

// Document ready handler
// Auto-resize input fields based on content
const $measureSpan = $('<span>')
  .addClass('measure-span')
  .appendTo('body');

function updateInputWidth(input) {
  const $input = $(input);
  const inputStyle = window.getComputedStyle(input);
  $measureSpan.css('font', inputStyle.font);

  const content = $input.val() || $input.attr('placeholder');
  $measureSpan.text(content);

  // Calculate desired width with padding
  const desiredWidth = $measureSpan.outerWidth() + 4; // Add 1px padding on each side + 1px border + 1px extra for text rendering
  
  // Get container width (using closest panel-body or container)
  const $container = $input.closest('.panel-body, .container');
  const containerWidth = $container.width();
  const maxWidth = Math.min(containerWidth * 0.9, 400); // 90% of container or 400px, whichever is smaller
  
  // Apply width, constrained by maxWidth
  $input.width(Math.min(desiredWidth, maxWidth));
}

// Setup input resize handling
function setupInputResize() {
  const $inputs = $('.message-section input[type="text"], #headers_list input[type="text"]');

  // Initial setup for existing inputs
  $inputs.each(function() {
    updateInputWidth(this);
  });

  // Handle all input events through delegation
  $('.container').on('input focus', 'input[type="text"]', function() {
    updateInputWidth(this);
  });

  // Handle placeholder changes using jQuery
  const $allTextInputs = $('input[type="text"]');

  // Create MutationObserver to watch for placeholder changes
  const observer = new MutationObserver(mutations => {
    mutations.forEach(mutation => {
      if (mutation.type === 'attributes' && mutation.attributeName === 'placeholder') {
        updateInputWidth(mutation.target);
      }
    });
  });

  // Apply observer to each input
  $allTextInputs.each(function() {
    observer.observe(this, {
      attributes: true,
      attributeFilter: ['placeholder']
    });
  });
}

$(function() {
  const templateName = new URLSearchParams(window.location.search).get('template');

  if (!templateName) {
    $('#template_error')
      .text('No template specified')
      .show();
    return;
  }

  // Store template name and update the panel title
  state = new TemplateState();
  state.templateName = templateName;
  $('#template_title').text(templateName);

  // Ensure template name is set
  if (!templateName) {
    $('#template_error')
      .text('No template specified')
      .show();
    return;
  }

  // Keep template name in state through template parsing
  const origTemplateName = templateName;

  // Fetch template content from backend with all query parameters
  const queryParams = new URLSearchParams(window.location.search);
  fetch(`/api/template?${queryParams}`)
    .then(response => {
      if (!response.ok) {
        throw new Error(`Failed to fetch template: ${response.statusText}`);
      }
      return response.text();
    })
    .then(async templateContent => {
      // Parse template and set up state
      state = parseTemplate(templateContent);
      state.templateName = origTemplateName; // Restore template name after parsing

      // Convert template to interactive HTML
      await templateToHtml(templateContent, state);
      setupInputResize();
      setupVariableHandlers();

      // Initial check of prompts
      checkAllPromptsFilled();

      // Add input handlers for prompt checking
      $('.container').on('input change', '[data-original-tag]', function() {
        checkAllPromptsFilled();
      });
    })
    .catch(err => {
      $('#template_error')
        .text(`Failed to process template: ${err.message}`)
        .fadeIn();
    });

  // Set up event handlers for inputs
  $('.container').on('input change', 'input, textarea, select', function() {
    const $this = $(this);
    const nodeName = $this.data('node');
    if (nodeName) {
      let value = $this.val();
      // Handle uppercase conversion if needed
      if ($this.data('uppercase')) {
        value = value.toUpperCase();
        $this.val(value); // Update the input with uppercase value
      }
      state.updateValue(nodeName, value);
    }
  });

  // Handle buttons
  $('#cancel').click(() => window.close());
  $('#save').click(() => submitTemplate());

  async function submitTemplate() {
    try {
      // Get form data but exclude template field
      const formData = {
        responses: state.collectFormData().responses
      };

      // Include all query parameters in submission URL
      const queryParams = new URLSearchParams(window.location.search);
      const response = await fetch(`/api/form?${queryParams}`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(formData)
      });

      if (!response.ok) {
        throw new Error(await response.text());
      }

      // Show success message
      $('#statusMessage')
        .removeClass('text-danger')
        .addClass('text-success')
        .text('Message posted to outbox successfully');

      // Close window immediately
      window.close();

    } catch (err) {
      $('#template_error')
        .text(`Failed to submit template: ${err.message}`)
        .show();
    }
  }
});
