// The shapes the mail API sends (internal/app/views.go).

export type Address = { name?: string; email: string };

export type Mailbox = { id: string; name: string; role: string; total: number; unread: number };

export type Account = {
	id: number;
	email: string;
	displayName: string;
	protocol: string;
	color: string;
	mailboxes: Mailbox[];
	error?: string;
};

export type Summary = {
	id: string;
	threadId: string;
	mailboxId: string;
	mailboxRole: string;
	accountId: number;
	from: Address[];
	to: Address[];
	cc: Address[];
	subject: string;
	preview: string;
	date: string;
	seen: boolean;
	flagged: boolean;
	answered: boolean;
	hasAttachment: boolean;
};

export type Attachment = { id: string; name: string; type: string; size: number; cid?: string; inline: boolean; url: string };

export type Message = Summary & {
	replyTo: Address[];
	bcc: Address[];
	attachments: Attachment[];
	body: { html: string; plain: boolean; hasRemote: boolean };
};

/** A row in the list: a conversation, or one message when threads are off. */
export type Item = { id: string; msgs: Summary[]; latest: Summary };

export type Prefs = {
	mode: 'light' | 'dark' | 'system';
	preset: 'slate' | 'paper' | 'fjord' | 'contrast';
	accent: string;
	density: 'compact' | 'comfortable' | 'spacious';
	layout: 'right' | 'below' | 'off';
	threads: boolean;
	rtl: boolean;
	listW: number;
	listH: number;
	rail: boolean;
	images: string[]; // senders whose images always show
};

export type Compose = {
	key: number;
	accountId: number;
	to: string;
	cc: string;
	bcc: string;
	subject: string;
	html: string;
	replyTo?: string;
	min: boolean;
	max: boolean;
	showCc: boolean;
};

export type Toast = { id: number; msg: string; undo?: () => void; error?: boolean };
