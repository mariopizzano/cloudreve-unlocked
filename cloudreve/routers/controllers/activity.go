package controllers

import (
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	activitysvc "github.com/cloudreve/Cloudreve/v4/service/activity"
	"github.com/gin-gonic/gin"
)

// AdminListActivities lists activity entries with admin filters.
func AdminListActivities(c *gin.Context) {
	service := ParametersFromContext[*activitysvc.AdminListActivitiesService](c, activitysvc.AdminListActivitiesParamCtx{})
	res, err := service.List(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}

	c.JSON(200, serializer.Response{Data: res})
}

// AdminGetActivity returns a single activity entry.
func AdminGetActivity(c *gin.Context) {
	service := ParametersFromContext[*activitysvc.AdminActivityDetailService](c, activitysvc.AdminActivityDetailParamCtx{})
	res, err := service.Get(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}

	c.JSON(200, serializer.Response{Data: res})
}

// UserListActivities lists the authenticated user's own activity.
func UserListActivities(c *gin.Context) {
	service := ParametersFromContext[*activitysvc.UserActivityListService](c, activitysvc.UserActivityListParamCtx{})
	res, err := service.List(c)
	if err != nil {
		c.JSON(200, serializer.Err(c, err))
		return
	}

	c.JSON(200, serializer.Response{Data: res})
}
