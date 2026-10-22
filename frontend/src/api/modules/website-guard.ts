import http from '@/api';
import { ResPage } from '../interface';
import { WebsiteGuard } from '../interface/website-guard';
import { TimeoutEnum } from '@/enums/http-enum';

export const getGuardStatus = () => {
    return http.get<WebsiteGuard.Status>(`/websites/guard/status`);
};

export const updateGuardGlobal = (req: WebsiteGuard.GlobalConfig) => {
    return http.post(`/websites/guard/global`, req, TimeoutEnum.T_60S);
};

export const operateGuardDetector = (req: WebsiteGuard.DetectorOperate) => {
    return http.post<string>(`/websites/guard/detector`, req, TimeoutEnum.T_60S);
};

export const getGuardSite = (id: number) => {
    return http.get<WebsiteGuard.SiteConfig>(`/websites/guard/site/${id}`);
};

export const updateGuardSite = (req: WebsiteGuard.SiteConfig) => {
    return http.post(`/websites/guard/site`, req, TimeoutEnum.T_60S);
};

export const searchGuardEvents = (req: WebsiteGuard.EventSearch) => {
    return http.post<ResPage<WebsiteGuard.Event>>(`/websites/guard/events`, req);
};

export const loadGuardEventStats = (req: WebsiteGuard.MonitorReq) => {
    return http.post<WebsiteGuard.EventStats>(`/websites/guard/events/stats`, req);
};

export const loadGuardCountries = () => {
    return http.get<WebsiteGuard.Country[]>(`/websites/guard/countries`);
};

export const loadMonitorOverview = (req: WebsiteGuard.MonitorReq) => {
    return http.post<WebsiteGuard.MonitorOverview>(`/websites/monitor/overview`, req);
};

export const loadMonitorTrend = (req: WebsiteGuard.MonitorReq) => {
    return http.post<WebsiteGuard.MonitorTrend>(`/websites/monitor/trend`, req);
};

export const loadMonitorTop = (req: WebsiteGuard.MonitorReq) => {
    return http.post<WebsiteGuard.MonitorTop>(`/websites/monitor/top`, req);
};

export const searchMonitorSlow = (req: WebsiteGuard.EventSearch) => {
    return http.post<ResPage<WebsiteGuard.Slow>>(`/websites/monitor/slow`, req);
};
