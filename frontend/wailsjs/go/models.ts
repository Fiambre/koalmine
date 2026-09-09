export namespace main {
	
	export class CreateTaskInput {
	    provider: string;
	    project: string;
	    title: string;
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateTaskInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.project = source["project"];
	        this.title = source["title"];
	        this.description = source["description"];
	    }
	}
	export class ProviderInfo {
	    name: string;
	    displayName: string;
	    fields: providers.ConfigField[];
	    enabled: boolean;
	    values: Record<string, string>;
	    secretsSet: Record<string, boolean>;
	    projectHint: string;
	
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
	        this.projectHint = source["projectHint"];
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
	
	export class Comment {
	    author: string;
	    body: string;
	    // Go type: time
	    createdAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Comment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.author = source["author"];
	        this.body = source["body"];
	        this.createdAt = this.convertValues(source["createdAt"], null);
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
	export class ProjectOption {
	    value: string;
	    label: string;
	
	    static createFrom(source: any = {}) {
	        return new ProjectOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.value = source["value"];
	        this.label = source["label"];
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
	    description: string;
	    createdByMe: boolean;
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
	        this.description = source["description"];
	        this.createdByMe = source["createdByMe"];
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

export namespace updater {
	
	export class Info {
	    available: boolean;
	    version: string;
	
	    static createFrom(source: any = {}) {
	        return new Info(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.version = source["version"];
	    }
	}

}

