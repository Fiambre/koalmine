export namespace main {
	
	export class ProviderInfo {
	    name: string;
	    displayName: string;
	    fields: providers.ConfigField[];
	    enabled: boolean;
	    values: Record<string, string>;
	    secretsSet: Record<string, boolean>;
	
	    static createFrom(source: any = {}) {
	        return new ProviderInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.displayName = source["displayName"];
	        this.fields = this.convertValues(source["fields"], providers.ConfigField);
	        this.enabled = source["enabled"];
	        this.values = source["values"];
	        this.secretsSet = source["secretsSet"];
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

export namespace providers {
	
	export class ConfigField {
	    key: string;
	    label: string;
	    kind: string;
	    placeholder: string;
	    required: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ConfigField(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.label = source["label"];
	        this.kind = source["kind"];
	        this.placeholder = source["placeholder"];
	        this.required = source["required"];
	    }
	}
	export class TaskItem {
	    id: string;
	    provider: string;
	    type: string;
	    title: string;
	    url: string;
	    project: string;
	    status: string;
	    author: string;
	    // Go type: time
	    updatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new TaskItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.provider = source["provider"];
	        this.type = source["type"];
	        this.title = source["title"];
	        this.url = source["url"];
	        this.project = source["project"];
	        this.status = source["status"];
	        this.author = source["author"];
	        this.updatedAt = this.convertValues(source["updatedAt"], null);
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

