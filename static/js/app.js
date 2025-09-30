document.addEventListener('DOMContentLoaded', function() {
  // Custom HTMX event handlers
  document.body.addEventListener('htmx:afterRequest', function(event) {
    // Add fade-in animation to newly loaded content
    const target = event.detail.target;
    if (target) {
      target.classList.add('fade-in');
    }
  });

  // Add loading indicator
  document.body.addEventListener('htmx:beforeRequest', function(event) {
    const trigger = event.detail.elt;
    if (trigger && trigger.tagName === 'BUTTON') {
      trigger.classList.add('loading');
    }
  });

  document.body.addEventListener('htmx:afterRequest', function(event) {
    const trigger = event.detail.elt;
    if (trigger && trigger.tagName === 'BUTTON') {
      trigger.classList.remove('loading');
    }
  });

  // Custom checkbox behavior
  const checkboxes = document.querySelectorAll('input[type="checkbox"]');
  checkboxes.forEach(checkbox => {
    checkbox.addEventListener('change', function() {
      // Add visual feedback
      const label = this.closest('label');
      if (this.checked) {
        label.classList.add('label-checked');
      } else {
        label.classList.remove('label-checked');
      }
    });

    // Initialize checked state on page load
    const label = checkbox.closest('label');
    if (checkbox.checked) {
      label.classList.add('label-checked');
    }
  });

  // Add smooth scrolling
  document.querySelectorAll('a[href^="#"]').forEach(anchor => {
    anchor.addEventListener('click', function(e) {
      e.preventDefault();
      const target = document.querySelector(this.getAttribute('href'));
      if (target) {
        target.scrollIntoView({ behavior: 'smooth' });
      }
    });
  });
});

// Theme switcher (optional)
function toggleTheme() {
  const html = document.querySelector('html');
  const currentTheme = html.getAttribute('data-theme');
  const newTheme = currentTheme === 'light' ? 'dark' : 'light';
  html.setAttribute('data-theme', newTheme);
  localStorage.setItem('theme', newTheme);
}

// Load saved theme
window.addEventListener('load', function() {
  const savedTheme = localStorage.getItem('theme') || 'light';
  document.querySelector('html').setAttribute('data-theme', savedTheme);
});
