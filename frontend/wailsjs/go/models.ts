export namespace main {
	
	export class ConnectionProfile {
	    name: string;
	    host: string;
	    port: string;
	    user: string;
	    password: string;
	    localPort: string;
	    remotePort: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.user = source["user"];
	        this.password = source["password"];
	        this.localPort = source["localPort"];
	        this.remotePort = source["remotePort"];
	    }
	}

}

