window.showToast = function showToast(message, kind) {
	const variant =
		{
			info: 'alert-info',
			success: 'alert-success',
			warning: 'alert-warning',
			error: 'alert-error',
		}[kind] || 'alert-info';

	const tag =
		{
			info: '[ INFO ]',
			success: '[ OK ]',
			warning: '[ WARN ]',
			error: '[ ERR ]',
		}[kind] || '[ INFO ]';

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

	const tagEl = document.createElement('span');
	tagEl.style.opacity = '0.75';
	tagEl.style.marginRight = '0.5rem';
	tagEl.textContent = tag;
	node.appendChild(tagEl);

	const msgEl = document.createElement('span');
	msgEl.textContent = message;
	node.appendChild(msgEl);

	host.appendChild(node);

	setTimeout(() => {
		node.style.transition = 'opacity 0.3s ease';
		node.style.opacity = '0';
		setTimeout(() => node.remove(), 300);
	}, 4000);
};
