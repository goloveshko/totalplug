export namespace domain {
	
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
	    web_url?: string;
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
	        this.web_url = source["web_url"];
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

}

