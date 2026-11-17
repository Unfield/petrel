import { i18n, t } from '@ternkit/tern';
import type { Address } from './types';

export const locale = () => (i18n.locale === 'de' ? 'de-DE' : 'en-US');

const sameDay = (a: Date, b: Date) => a.toDateString() === b.toDateString();

/** A date for the list: the time today, "Yesterday", the weekday, or the date. */
export function fmtShort(iso: string): string {
	const d = new Date(iso);
	const now = new Date();
	const L = locale();
	if (sameDay(d, now)) return d.toLocaleTimeString(L, { hour: 'numeric', minute: '2-digit' });
	const y = new Date(now);
	y.setDate(now.getDate() - 1);
	if (sameDay(d, y)) {
		const s = new Intl.RelativeTimeFormat(L, { numeric: 'auto' }).format(-1, 'day');
		return s.charAt(0).toUpperCase() + s.slice(1);
	}
	if (+now - +d < 6 * 864e5) return d.toLocaleDateString(L, { weekday: 'short' });
	if (d.getFullYear() === now.getFullYear()) return d.toLocaleDateString(L, { month: 'short', day: 'numeric' });
	return d.toLocaleDateString(L, { year: 'numeric', month: 'short', day: 'numeric' });
}

/** A date for the reading pane, with how long ago when it's recent. */
export function fmtLong(iso: string): string {
	const d = new Date(iso);
	const L = locale();
	const diff = (Date.now() - +d) / 60000;
	const abs = d.toLocaleString(L, { weekday: 'short', month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' });
	const rtf = new Intl.RelativeTimeFormat(L, { numeric: 'auto' });
	let rel = '';
	if (diff < 60) rel = rtf.format(-Math.max(1, Math.round(diff)), 'minute');
	else if (diff < 1440) rel = rtf.format(-Math.round(diff / 60), 'hour');
	else if (diff < 10080) rel = rtf.format(-Math.round(diff / 1440), 'day');
	return rel ? `${abs} (${rel})` : abs;
}

export function fmtSize(n: number): string {
	const f = new Intl.NumberFormat(locale(), { maximumFractionDigits: 1 });
	if (n < 1024) return `${n} B`;
	if (n < 1024 * 1024) return `${f.format(n / 1024)} KB`;
	if (n < 1024 ** 3) return `${f.format(n / 1024 / 1024)} MB`;
	return `${f.format(n / 1024 ** 3)} GB`;
}

export function hue(s: string): number {
	let h = 0;
	for (const c of s) h = (h * 31 + c.charCodeAt(0)) % 360;
	return h;
}

export function initials(name: string): string {
	const words = name.split(/[\s.@_-]+/).filter((w) => /\p{L}/u.test(w[0] ?? ''));
	return (
		words
			.map((w) => w[0])
			.slice(0, 2)
			.join('')
			.toUpperCase() || '?'
	);
}

export const display = (a: Address | undefined): string => (a ? a.name || a.email.split('@')[0] : '');
export const first = (a: Address | undefined): string => display(a).split(' ')[0];

/** Accent text color: dark on light accents, white on dark ones. */
export function accentFg(hex: string): string {
	const n = parseInt(hex.slice(1), 16);
	const lum = (0.2126 * ((n >> 16) & 255) + 0.7152 * ((n >> 8) & 255) + 0.0722 * (n & 255)) / 255;
	return lum > 0.62 ? '#111418' : '#ffffff';
}

export const isMac = () => typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform || navigator.userAgent);

export function quoteHeader(date: string, name: string) {
	return t('m.onWrote', { date: new Date(date).toLocaleString(locale(), { dateStyle: 'medium', timeStyle: 'short' }), name });
}

export const escapeHTML = (s: string) => s.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]!);
