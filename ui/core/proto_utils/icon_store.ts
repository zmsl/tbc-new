import { IconData } from '../proto/ui.js';

// Wowhead tooltip data for TBC does not change: an item's icon and a spell's rank are fixed
// properties of an expansion that shipped in 2007. The in-memory cache in database.ts dies with
// the page, so every reload re-fetches every id the local database does not already cover.
// This keeps them in IndexedDB instead, where they survive until the schema changes.
//
// Every operation here degrades to a miss rather than an error. A private window, site data
// blocked by policy, a browser that refuses to open the database at all -- in each case the
// caller ends up doing exactly what it did before this existed.

const DB_NAME = 'wowsims-icons';
const STORE_NAME = 'tooltips';
// Bump to discard everything cached under an older shape. IndexedDB uses this as its version,
// so raising it drops and recreates the store.
const SCHEMA_VERSION = 1;

type StoredIcon = {
	id: number;
	name: string;
	icon: string;
	hasBuff: boolean;
	rank: number;
};

let dbPromise: Promise<IDBDatabase | null> | null = null;

function openDatabase(): Promise<IDBDatabase | null> {
	if (dbPromise) return dbPromise;

	dbPromise = new Promise<IDBDatabase | null>(resolve => {
		let request: IDBOpenDBRequest;
		try {
			// Accessing indexedDB itself throws in some locked-down contexts, so even this is guarded.
			request = indexedDB.open(DB_NAME, SCHEMA_VERSION);
		} catch {
			resolve(null);
			return;
		}

		request.onupgradeneeded = () => {
			const db = request.result;
			// Recreated rather than migrated: this is a cache of data that can always be fetched
			// again, so throwing it away is cheaper than reshaping it.
			if (db.objectStoreNames.contains(STORE_NAME)) db.deleteObjectStore(STORE_NAME);
			db.createObjectStore(STORE_NAME);
		};
		request.onsuccess = () => resolve(request.result);
		request.onerror = () => resolve(null);
		request.onblocked = () => resolve(null);
	});

	return dbPromise;
}

export async function readCachedIcon(key: string): Promise<IconData | null> {
	const db = await openDatabase();
	if (!db) return null;

	return new Promise<IconData | null>(resolve => {
		try {
			const request = db.transaction(STORE_NAME, 'readonly').objectStore(STORE_NAME).get(key);
			request.onsuccess = () => {
				const stored = request.result as StoredIcon | undefined;
				resolve(stored ? IconData.create(stored) : null);
			};
			request.onerror = () => resolve(null);
		} catch {
			resolve(null);
		}
	});
}

/**
 * Stores one tooltip result. Fire and forget: a write that fails costs a future lookup, not
 * correctness, and nothing waiting on an icon should wait on the disk too.
 */
export function writeCachedIcon(key: string, data: IconData): void {
	// A result without an icon means the fetch failed or the id does not exist. Caching that
	// would turn a transient Wowhead outage into a permanently broken icon for this user.
	if (!data.icon) return;

	openDatabase().then(db => {
		if (!db) return;
		try {
			const stored: StoredIcon = {
				id: data.id,
				name: data.name,
				icon: data.icon,
				hasBuff: data.hasBuff,
				rank: data.rank,
			};
			db.transaction(STORE_NAME, 'readwrite').objectStore(STORE_NAME).put(stored, key);
		} catch {
			// Quota exceeded, or the store vanished under us. Nothing to do and nothing to report.
		}
	});
}
