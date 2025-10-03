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

// Theme toggle initialization for use after dynamic page loads (like preview page)
function initThemeToggle() {
  const themeToggle = document.getElementById('theme-toggle');
  const currentTheme = localStorage.getItem('theme') || 'dark';

  if (themeToggle) {
    // Set checkbox state
    if (currentTheme === 'light') {
      themeToggle.checked = true;
    } else {
      themeToggle.checked = false;
    }

    // Add event listener
    themeToggle.addEventListener('change', function() {
      if (this.checked) {
        document.documentElement.setAttribute('data-theme', 'light');
        localStorage.setItem('theme', 'light');
      } else {
        document.documentElement.setAttribute('data-theme', 'dark');
        localStorage.setItem('theme', 'dark');
      }
    });
  }
}
