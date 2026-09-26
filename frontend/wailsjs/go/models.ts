export namespace domain {
	
	export class AppInfo {
	    version: string;
	    releases_url: string;
	    data_dir: string;
	    go_version: string;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.releases_url = source["releases_url"];
	        this.data_dir = source["data_dir"];
	        this.go_version = source["go_version"];
	    }
	}
	export class AppUpdate {
	    current: string;
	    latest?: string;
	    available: boolean;
	    comparable: boolean;
	    release_url?: string;
	    // Go type: time
	    checked_at?: any;
	
	    static createFrom(source: any = {}) {
	        return new AppUpdate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current = source["current"];
	        this.latest = source["latest"];
	        this.available = source["available"];
	        this.comparable = source["comparable"];
	        this.release_url = source["release_url"];
	        this.checked_at = this.convertValues(source["checked_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Override {
	    custom_name?: string;
	    custom_version?: string;
	    custom_source?: string;
	    ignore_updates?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Override(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.custom_name = source["custom_name"];
	        this.custom_version = source["custom_version"];
	        this.custom_source = source["custom_source"];
	        this.ignore_updates = source["ignore_updates"];
	    }
	}
	export class Plugin {
	    id: string;
	    name: string;
	    type: string;
	    path: string;
	    installed_version: string;
	    latest_version?: string;
	    confidence: string;
	    custom_source?: string;
	    file_hash?: string;
	    download_url?: string;
	    download_urls?: string[];
	    web_url?: string;
	    source?: string;
	    arch?: string;
	    description?: string;
	    status: string;
	    // Go type: time
	    last_checked?: any;
	
	    static createFrom(source: any = {}) {
	        return new Plugin(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.type = source["type"];
	        this.path = source["path"];
	        this.installed_version = source["installed_version"];
	        this.latest_version = source["latest_version"];
	        this.confidence = source["confidence"];
	        this.custom_source = source["custom_source"];
	        this.file_hash = source["file_hash"];
	        this.download_url = source["download_url"];
	        this.download_urls = source["download_urls"];
	        this.web_url = source["web_url"];
	        this.source = source["source"];
	        this.arch = source["arch"];
	        this.description = source["description"];
	        this.status = source["status"];
	        this.last_checked = this.convertValues(source["last_checked"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class UpdateEntry {
	    plugin_id: string;
	    plugin_name: string;
	    version: string;
	    // Go type: time
	    applied_at: any;
	
	    static createFrom(source: any = {}) {
	        return new UpdateEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.plugin_id = source["plugin_id"];
	        this.plugin_name = source["plugin_name"];
	        this.version = source["version"];
	        this.applied_at = this.convertValues(source["applied_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class UpdateResult {
	    locked: boolean;
	    backup_path?: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.locked = source["locked"];
	        this.backup_path = source["backup_path"];
	    }
	}

}

