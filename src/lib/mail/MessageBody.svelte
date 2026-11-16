<script lang="ts">
	// A message's body, in a frame of its own: sandboxed (no scripts), with a
	// Content-Security-Policy that loads no remote content until the reader
	// allows images. Links open in a new tab, outside the sandbox.
	let { html, plain, images, messageId }: { html: string; plain: boolean; images: boolean; messageId: string } = $props();

	let frame = $state<HTMLIFrameElement>();
	let height = $state(60);

	const doc = $derived.by(() => {
		const origin = typeof location === 'undefined' ? '' : location.origin;
		// Inline images (cid:) come from Petrel's attachment endpoint.
		const body = html.replace(
			/(src\s*=\s*["']?)cid:([^"'\s>]+)/gi,
			(_, pre, cid) => `${pre}${origin}/api/mail/attachment?id=${encodeURIComponent(messageId)}&part=${encodeURIComponent('cid:' + cid)}`
		);
		const remote = images ? ' https: http:' : '';
		const csp = `default-src 'none'; style-src 'unsafe-inline'; img-src data: ${origin}${remote}; font-src data:${remote}; media-src 'none'`;
		// Plain text takes the app's colors; HTML mail keeps its own, on white.
		const base = plain
			? `body{margin:0;font:15px/1.62 ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;color:var(--fg);background:transparent;overflow-wrap:anywhere}
			   a{color:var(--link)} blockquote{margin:.6em 0;padding-inline-start:12px;border-inline-start:3px solid var(--line);color:var(--muted)}
			   .petrel-plain{white-space:normal}`
			: `body{margin:0;padding:16px;font:15px/1.5 ui-sans-serif,system-ui,-apple-system,"Segoe UI",Roboto,sans-serif;color:#1f2328;background:#fff;overflow-wrap:anywhere}
			   img{max-width:100%;height:auto} table{max-width:100%!important} pre{white-space:pre-wrap}`;
		return `<!doctype html><html><head><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="${csp}"><base target="_blank"><style>${base}</style></head><body>${body}</body></html>`;
	});

	function measure() {
		const d = frame?.contentDocument;
		if (!d?.documentElement) return;
		height = Math.max(40, d.documentElement.scrollHeight);
	}

	function setup() {
		const d = frame?.contentDocument;
		if (!d) return;
		if (plain) {
			// Hand the theme's colors to the frame.
			const cs = getComputedStyle(frame!);
			d.documentElement.style.setProperty('--fg', cs.getPropertyValue('--text'));
			d.documentElement.style.setProperty('--muted', cs.getPropertyValue('--text-2'));
			d.documentElement.style.setProperty('--line', cs.getPropertyValue('--border'));
			d.documentElement.style.setProperty('--link', cs.getPropertyValue('--accent-ink'));
			d.documentElement.style.colorScheme = getComputedStyle(frame!).colorScheme;
		}
		for (const a of d.querySelectorAll('a[href]')) a.setAttribute('rel', 'noopener noreferrer');
		measure();
		new ResizeObserver(measure).observe(d.documentElement);
		for (const img of d.images) img.addEventListener('load', measure);
	}
</script>

<iframe
	bind:this={frame}
	class="mbody"
	class:paper={!plain}
	title="Message"
	srcdoc={doc}
	sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox"
	referrerpolicy="no-referrer"
	style:height="{height}px"
	onload={setup}
></iframe>
