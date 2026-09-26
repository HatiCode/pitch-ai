// Event IDs are generated on the device, before anything reaches a server, so
// two coaches tagging one match must not be able to collide. UUIDv7 puts the
// timestamp in the high bits, which also makes the IDs sort in the order they
// were tapped — the property the fold leans on to order a log assembled from
// more than one device.

const TIMESTAMP_BYTES = 6;
const UUID_BYTES = 16;

// MAX_COUNTER is what fits in the twelve bits after the version nibble.
const MAX_COUNTER = 0xfff;

// counter disambiguates IDs generated inside one millisecond, and lastTimestamp
// is the millisecond they are being stamped with — which is not always the
// current one, see below. Both reset on page load, which is fine: the random
// tail keeps IDs unique, and ordering only has to be consistent within a run,
// not reproducible across them.
let lastTimestamp = -1;
let counter = 0;

// nextStamp advances the clock the IDs are built from, never letting it move
// backwards.
//
// Two things would otherwise break ordering. A burst of more than 4096 taps
// inside one millisecond overflows the counter, so it borrows a millisecond
// from the future instead of wrapping. And a clock correction mid-match — NTP,
// a timezone change, a coach fixing the iPad's time — would rewind every ID
// generated after it, so a backwards jump is ignored outright. Both cost
// nothing but a timestamp slightly ahead of the wall clock, which no reader of
// these IDs cares about: the event's own clockMs is what a coach ever sees.
function nextStamp(): number {
	const now = Date.now();
	if (now > lastTimestamp) {
		lastTimestamp = now;
		counter = 0;
		return lastTimestamp;
	}
	if (counter >= MAX_COUNTER) {
		lastTimestamp++;
		counter = 0;
		return lastTimestamp;
	}
	counter++;
	return lastTimestamp;
}

const hex = Array.from({ length: 256 }, (_, byte) =>
	byte.toString(16).padStart(2, "0"),
);

export function uuidv7(): string {
	const bytes = new Uint8Array(UUID_BYTES);
	crypto.getRandomValues(bytes);

	const timestamp = nextStamp();

	// 48 bits of Unix milliseconds, most significant byte first.
	for (let i = TIMESTAMP_BYTES - 1; i >= 0; i--) {
		bytes[i] = Math.floor(timestamp / 256 ** (TIMESTAMP_BYTES - 1 - i)) & 0xff;
	}

	// Version 7 in the high nibble of byte 6, then the counter across the twelve
	// bits that follow it.
	bytes[6] = 0x70 | ((counter >>> 8) & 0x0f);
	bytes[7] = counter & 0xff;

	// RFC 9562 variant: the top two bits of byte 8 are 10.
	bytes[8] = (bytes[8] & 0x3f) | 0x80;

	const s = Array.from(bytes, (byte) => hex[byte]).join("");
	return `${s.slice(0, 8)}-${s.slice(8, 12)}-${s.slice(12, 16)}-${s.slice(16, 20)}-${s.slice(20)}`;
}
