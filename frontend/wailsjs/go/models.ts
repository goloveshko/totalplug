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
	    checked_at?: string;
	
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
	        this.checked_at = source["checked_at"];
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
	    last_checked?: string;
	
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
	        this.last_checked = source["last_checked"];
	    }
	}
	export class TcConfig {
	    configured: string;
	    detected: string;
	    effective: string;
	    readable: boolean;
	
	    static createFrom(source: any = {}) {
	        return new TcConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.configured = source["configured"];
	        this.detected = source["detected"];
	        this.effective = source["effective"];
	        this.readable = source["readable"];
	    }
	}
	export class UpdateEntry {
	    plugin_id: string;
	    plugin_name: string;
	    version: string;
	    applied_at: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.plugin_id = source["plugin_id"];
	        this.plugin_name = source["plugin_name"];
	        this.version = source["version"];
	        this.applied_at = source["applied_at"];
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

