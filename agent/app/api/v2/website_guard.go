package v2

import (
	"github.com/1Panel-dev/1Panel/agent/app/api/v2/helper"
	"github.com/1Panel-dev/1Panel/agent/app/dto"
	"github.com/gin-gonic/gin"
)

// @Tags Website Guard
// @Summary Load website guard status
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/guard/status [get]
func (b *BaseApi) GetGuardStatus(c *gin.Context) {
	res, err := websiteGuardService.GetStatus()
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

// @Tags Website Guard
// @Summary Update website guard global config
// @Accept json
// @Param request body dto.GuardGlobalConfig true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/guard/global [post]
// @x-panel-log {"bodyKeys":[],"paramKeys":[],"BeforeFunctions":[],"formatZH":"更新网站防护全局配置","formatEN":"Update website guard global config"}
func (b *BaseApi) UpdateGuardGlobal(c *gin.Context) {
	var req dto.GuardGlobalConfig
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := websiteGuardService.UpdateGlobal(req); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

// @Tags Website Guard
// @Summary Operate SafeLine detector
// @Accept json
// @Param request body dto.GuardDetectorOperate true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/guard/detector [post]
// @x-panel-log {"bodyKeys":["operate"],"paramKeys":[],"BeforeFunctions":[],"formatZH":"[operate] 防护检测引擎","formatEN":"[operate] guard detector"}
func (b *BaseApi) OperateGuardDetector(c *gin.Context) {
	var req dto.GuardDetectorOperate
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	res, err := websiteGuardService.OperateDetector(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

// @Tags Website Guard
// @Summary Load website guard config of a site
// @Param id path integer true "website id"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/guard/site/{id} [get]
func (b *BaseApi) GetGuardSite(c *gin.Context) {
	id, err := helper.GetParamID(c)
	if err != nil {
		helper.BadRequest(c, err)
		return
	}
	res, err := websiteGuardService.GetSite(id)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

// @Tags Website Guard
// @Summary Update website guard config of a site
// @Accept json
// @Param request body dto.GuardSiteConfig true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/guard/site [post]
// @x-panel-log {"bodyKeys":["websiteId"],"paramKeys":[],"BeforeFunctions":[{"input_column":"id","input_value":"websiteId","isList":false,"db":"websites","output_column":"primary_domain","output_value":"domain"}],"formatZH":"更新网站 [domain] 防护配置","formatEN":"Update guard config of website [domain]"}
func (b *BaseApi) UpdateGuardSite(c *gin.Context) {
	var req dto.GuardSiteConfig
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	if err := websiteGuardService.UpdateSite(req); err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.Success(c)
}

// @Tags Website Guard
// @Summary Search website guard events
// @Accept json
// @Param request body dto.GuardEventSearch true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/guard/events [post]
func (b *BaseApi) SearchGuardEvents(c *gin.Context) {
	var req dto.GuardEventSearch
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	total, list, err := websiteGuardService.SearchEvents(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, dto.PageResult{Total: total, Items: list})
}

// @Tags Website Guard
// @Summary Load website guard event stats
// @Accept json
// @Param request body dto.WebsiteMonitorReq true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/guard/events/stats [post]
func (b *BaseApi) LoadGuardEventStats(c *gin.Context) {
	var req dto.WebsiteMonitorReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	res, err := websiteGuardService.LoadEventStats(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

// @Tags Website Guard
// @Summary Load countries for region blocking
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/guard/countries [get]
func (b *BaseApi) LoadGuardCountries(c *gin.Context) {
	res, err := websiteGuardService.LoadCountries()
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

// @Tags Website Guard
// @Summary Load website monitor overview
// @Accept json
// @Param request body dto.WebsiteMonitorReq true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/monitor/overview [post]
func (b *BaseApi) LoadWebsiteMonitorOverview(c *gin.Context) {
	var req dto.WebsiteMonitorReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	res, err := websiteGuardService.LoadMonitorOverview(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

// @Tags Website Guard
// @Summary Load website monitor trend
// @Accept json
// @Param request body dto.WebsiteMonitorReq true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/monitor/trend [post]
func (b *BaseApi) LoadWebsiteMonitorTrend(c *gin.Context) {
	var req dto.WebsiteMonitorReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	res, err := websiteGuardService.LoadMonitorTrend(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

// @Tags Website Guard
// @Summary Load website monitor rankings
// @Accept json
// @Param request body dto.WebsiteMonitorReq true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/monitor/top [post]
func (b *BaseApi) LoadWebsiteMonitorTop(c *gin.Context) {
	var req dto.WebsiteMonitorReq
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	res, err := websiteGuardService.LoadMonitorTop(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, res)
}

// @Tags Website Guard
// @Summary Search slow requests
// @Accept json
// @Param request body dto.GuardEventSearch true "request"
// @Success 200
// @Security ApiKeyAuth
// @Security Timestamp
// @Router /websites/monitor/slow [post]
func (b *BaseApi) SearchWebsiteMonitorSlow(c *gin.Context) {
	var req dto.GuardEventSearch
	if err := helper.CheckBindAndValidate(&req, c); err != nil {
		return
	}
	total, list, err := websiteGuardService.SearchMonitorSlow(req)
	if err != nil {
		helper.InternalServer(c, err)
		return
	}
	helper.SuccessWithData(c, dto.PageResult{Total: total, Items: list})
}
