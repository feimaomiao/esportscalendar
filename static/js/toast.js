window.showToast = function showToast(message, kind) {
	const variant = ({
		info: 'alert-info',
		success: 'alert-success',
		warning: 'alert-warning',
		error: 'alert-error',
	})[kind] || 'alert-info';

	let host = document.getElementById('toast-host');
	if (!host) {
		host = document.createElement('div');
		host.id = 'toast-host';
		host.className = 'toast toast-end z-50';
		document.body.appendChild(host);
	}

	const node = document.createElement('div');
	node.className = `alert ${variant} shadow-lg`;
	node.setAttribute('role', kind === 'error' ? 'alert' : 'status');
	node.textContent = message;
	host.appendChild(node);

	setTimeout(() => {
		node.style.transition = 'opacity 0.3s ease';
		node.style.opacity = '0';
		setTimeout(() => node.remove(), 300);
	}, 4000);
};
