export namespace config {
	
	export class BackupConfig {
	    max_concurrency: number;
	    min_free_disk_space_gb: number;
	    max_metadata_size_mb: number;
	    max_storage_gb: number;
	    storage_warning_pct: number;
	    sync_owned: boolean;
	    sync_created: boolean;
	
	    static createFrom(source: any = {}) {
	        return new BackupConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.max_concurrency = source["max_concurrency"];
	        this.min_free_disk_space_gb = source["min_free_disk_space_gb"];
	        this.max_metadata_size_mb = source["max_metadata_size_mb"];
	        this.max_storage_gb = source["max_storage_gb"];
	        this.storage_warning_pct = source["storage_warning_pct"];
	        this.sync_owned = source["sync_owned"];
	        this.sync_created = source["sync_created"];
	    }
	}
	export class TZKTConfig {
	    BaseURL: string;
	
	    static createFrom(source: any = {}) {
	        return new TZKTConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.BaseURL = source["BaseURL"];
	    }
	}
	export class ServerConfig {
	    BindAddress: string;
	    EnableAuth: boolean;
	    AuthUser: string;
	    AuthPass: string;
	
	    static createFrom(source: any = {}) {
	        return new ServerConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.BindAddress = source["BindAddress"];
	        this.EnableAuth = source["EnableAuth"];
	        this.AuthUser = source["AuthUser"];
	        this.AuthPass = source["AuthPass"];
	    }
	}
	export class IPFSConfig {
	    repo_path: string;
	    max_file_size: number;
	    pin_timeout: number;
	    rate_limit_mbps: number;
	
	    static createFrom(source: any = {}) {
	        return new IPFSConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.repo_path = source["repo_path"];
	        this.max_file_size = source["max_file_size"];
	        this.pin_timeout = source["pin_timeout"];
	        this.rate_limit_mbps = source["rate_limit_mbps"];
	    }
	}
	export class Config {
	    IPFS: IPFSConfig;
	    Server: ServerConfig;
	    Backup: BackupConfig;
	    TZKT: TZKTConfig;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.IPFS = this.convertValues(source["IPFS"], IPFSConfig);
	        this.Server = this.convertValues(source["Server"], ServerConfig);
	        this.Backup = this.convertValues(source["Backup"], BackupConfig);
	        this.TZKT = this.convertValues(source["TZKT"], TZKTConfig);
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

export namespace core {
	
	export class ServiceStatus {
	    state: string;
	    message: string;
	    is_paused: boolean;
	    current_wallet: string;
	    wallets_total: number;
	    wallets_synced: number;
	    total_nfts: number;
	    processed_nfts: number;
	    total_assets: number;
	    pinned_assets: number;
	    failed_assets: number;
	    pending_retries: number;
	    current_item: string;
	    // Go type: time
	    last_sync_at?: any;
	
	    static createFrom(source: any = {}) {
	        return new ServiceStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.message = source["message"];
	        this.is_paused = source["is_paused"];
	        this.current_wallet = source["current_wallet"];
	        this.wallets_total = source["wallets_total"];
	        this.wallets_synced = source["wallets_synced"];
	        this.total_nfts = source["total_nfts"];
	        this.processed_nfts = source["processed_nfts"];
	        this.total_assets = source["total_assets"];
	        this.pinned_assets = source["pinned_assets"];
	        this.failed_assets = source["failed_assets"];
	        this.pending_retries = source["pending_retries"];
	        this.current_item = source["current_item"];
	        this.last_sync_at = this.convertValues(source["last_sync_at"], null);
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

export namespace db {
	
	export class NFT {
	    id: number;
	    token_id: string;
	    contract_address: string;
	    wallet_address: string;
	    name: string;
	    description: string;
	    creator: string;
	    artifact_uri: string;
	    display_uri: string;
	    thumbnail_uri: string;
	    raw_metadata: string;
	    assets?: Asset[];
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new NFT(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.token_id = source["token_id"];
	        this.contract_address = source["contract_address"];
	        this.wallet_address = source["wallet_address"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.creator = source["creator"];
	        this.artifact_uri = source["artifact_uri"];
	        this.display_uri = source["display_uri"];
	        this.thumbnail_uri = source["thumbnail_uri"];
	        this.raw_metadata = source["raw_metadata"];
	        this.assets = this.convertValues(source["assets"], Asset);
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
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
	export class Asset {
	    id: number;
	    uri: string;
	    nft_id: number;
	    nft?: NFT;
	    type: string;
	    mime_type: string;
	    status: string;
	    error_msg: string;
	    size_bytes: number;
	    retry_count: number;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    pinned_at?: any;
	
	    static createFrom(source: any = {}) {
	        return new Asset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.uri = source["uri"];
	        this.nft_id = source["nft_id"];
	        this.nft = this.convertValues(source["nft"], NFT);
	        this.type = source["type"];
	        this.mime_type = source["mime_type"];
	        this.status = source["status"];
	        this.error_msg = source["error_msg"];
	        this.size_bytes = source["size_bytes"];
	        this.retry_count = source["retry_count"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.pinned_at = this.convertValues(source["pinned_at"], null);
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
	
	export class Wallet {
	    address: string;
	    alias: string;
	    type: string;
	    sync_owned: boolean;
	    sync_created: boolean;
	    // Go type: time
	    last_synced_at?: any;
	    last_synced_level: number;
	    // Go type: time
	    last_updated: any;
	    nfts?: NFT[];
	
	    static createFrom(source: any = {}) {
	        return new Wallet(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.address = source["address"];
	        this.alias = source["alias"];
	        this.type = source["type"];
	        this.sync_owned = source["sync_owned"];
	        this.sync_created = source["sync_created"];
	        this.last_synced_at = this.convertValues(source["last_synced_at"], null);
	        this.last_synced_level = source["last_synced_level"];
	        this.last_updated = this.convertValues(source["last_updated"], null);
	        this.nfts = this.convertValues(source["nfts"], NFT);
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

export namespace ipfs {
	
	export class VerifyResult {
	    cid: string;
	    is_pinned: boolean;
	    is_available: boolean;
	    size: number;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new VerifyResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cid = source["cid"];
	        this.is_pinned = source["is_pinned"];
	        this.is_available = source["is_available"];
	        this.size = source["size"];
	        this.error = source["error"];
	    }
	}

}

export namespace main {
	
	export class StorageInfo {
	    used_bytes: number;
	    used_gb: number;
	    disk_usage_bytes: number;
	    disk_usage_gb: number;
	    max_storage_gb: number;
	    warning_pct: number;
	    usage_pct: number;
	    is_warning: boolean;
	    is_limit_reached: boolean;
	    free_disk_space_gb: number;
	    repo_path: string;
	
	    static createFrom(source: any = {}) {
	        return new StorageInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.used_bytes = source["used_bytes"];
	        this.used_gb = source["used_gb"];
	        this.disk_usage_bytes = source["disk_usage_bytes"];
	        this.disk_usage_gb = source["disk_usage_gb"];
	        this.max_storage_gb = source["max_storage_gb"];
	        this.warning_pct = source["warning_pct"];
	        this.usage_pct = source["usage_pct"];
	        this.is_warning = source["is_warning"];
	        this.is_limit_reached = source["is_limit_reached"];
	        this.free_disk_space_gb = source["free_disk_space_gb"];
	        this.repo_path = source["repo_path"];
	    }
	}

}

export namespace storage {
	
	export class MigrationStatus {
	    in_progress: boolean;
	    source_path: string;
	    dest_path: string;
	    progress: number;
	    bytes_copied: number;
	    total_bytes: number;
	    current_file: string;
	    error?: string;
	    method: string;
	    phase: string;
	
	    static createFrom(source: any = {}) {
	        return new MigrationStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.in_progress = source["in_progress"];
	        this.source_path = source["source_path"];
	        this.dest_path = source["dest_path"];
	        this.progress = source["progress"];
	        this.bytes_copied = source["bytes_copied"];
	        this.total_bytes = source["total_bytes"];
	        this.current_file = source["current_file"];
	        this.error = source["error"];
	        this.method = source["method"];
	        this.phase = source["phase"];
	    }
	}
	export class StorageLocation {
	    path: string;
	    type: string;
	    label: string;
	    total_bytes: number;
	    free_bytes: number;
	    is_writable: boolean;
	    is_mounted: boolean;
	    mount_point: string;
	    network_uri: string;
	
	    static createFrom(source: any = {}) {
	        return new StorageLocation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.type = source["type"];
	        this.label = source["label"];
	        this.total_bytes = source["total_bytes"];
	        this.free_bytes = source["free_bytes"];
	        this.is_writable = source["is_writable"];
	        this.is_mounted = source["is_mounted"];
	        this.mount_point = source["mount_point"];
	        this.network_uri = source["network_uri"];
	    }
	}

}

