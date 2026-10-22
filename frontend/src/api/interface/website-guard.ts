import { ReqPage } from '.';

export namespace WebsiteGuard {
    export interface GlobalConfig {
        enable: boolean;
        ipAllow: string[];
        ipDeny: string[];
        geoMode: string;
        countries: string[];
        slowThreshold: number;
        minuteDays: number;
        dayDays: number;
        eventDays: number;
        mode: string;
        ccEnable: boolean;
        ccDuration: number;
        ccThreshold: number;
        ccBanTime: number;
    }

    export interface SiteBrief {
        websiteId: number;
        alias: string;
        primaryDomain: string;
        enable: boolean;
        mode: string;
    }

    export interface Status {
        openrestyInstalled: boolean;
        openrestyRunning: boolean;
        wafConflict: boolean;
        detectorInstalled: boolean;
        detectorStatus: string;
        mgtStatus: string;
        consolePort: number;
        safelineVersion: string;
        config: GlobalConfig;
        sites: SiteBrief[];
    }

    export interface SiteConfig {
        websiteId: number;
        enable: boolean;
        mode: string;
        ccEnable: boolean;
        ccDuration: number;
        ccThreshold: number;
        ccBanTime: number;
        ipAllow: string[];
        ipDeny: string[];
        geoMode: string;
        countries: string[];
    }

    export interface DetectorOperate {
        operate: string;
        taskID?: string;
        consolePort?: number;
    }

    export interface EventSearch extends ReqPage {
        site?: string;
        type?: string;
        ip?: string;
        startTime?: number;
        endTime?: number;
    }

    export interface Event {
        id: number;
        site: string;
        time: number;
        ip: string;
        country: string;
        method: string;
        host: string;
        uri: string;
        type: string;
        action: string;
        eventId: string;
        ua: string;
    }

    export interface Count {
        name: string;
        count: number;
    }

    export interface EventStats {
        total: number;
        byType: Record<string, number>;
        bySite: Record<string, number>;
        topIP: Count[];
        trend: Count[];
    }

    export interface Country {
        code: string;
        zh: string;
        en: string;
    }

    export interface MonitorReq {
        site?: string;
        startTime: number;
        endTime: number;
    }

    export interface MonitorOverview {
        requests: number;
        uv: number;
        ip: number;
        bytesIn: number;
        bytesOut: number;
        s2xx: number;
        s3xx: number;
        s4xx: number;
        s5xx: number;
        blocked: number;
        avgRt: number;
        p95Rt: number;
        peakQps: number;
    }

    export interface MonitorPoint {
        time: number;
        requests: number;
        uv: number;
        ip: number;
        bytesIn: number;
        bytesOut: number;
        s2xx: number;
        s3xx: number;
        s4xx: number;
        s5xx: number;
        blocked: number;
        qps: number;
        avgRt: number;
        p95Rt: number;
    }

    export interface MonitorTrend {
        interval: number;
        points: MonitorPoint[];
    }

    export interface MonitorTop {
        url: Count[];
        ip: Count[];
        referer: Count[];
        ua: Count[];
        country: Count[];
        errUrl: Count[];
    }

    export interface Slow {
        id: number;
        site: string;
        time: number;
        method: string;
        uri: string;
        ip: string;
        status: number;
        requestTime: number;
    }
}
