import { Box, Divider, FormControlLabel, Stack, Switch, Typography } from "@mui/material";
import { useContext } from "react";
import { useTranslation } from "react-i18next";
import { NoMarginHelperText, SettingSection } from "../Settings";
import { SettingContext } from "../SettingWrapper";

// Categories of activities that are captured when activity logging is enabled.
const activityCategories = [
  "settings.userEvents",
  "settings.fileEvents",
  "settings.shareEvents",
  "settings.systemEvents",
];

const Events = () => {
  const { t } = useTranslation("dashboard");
  const { setSettings, values } = useContext(SettingContext);
  const enabled = values["activity_log_enabled"] !== "0";

  return (
    <Box component={"form"} onSubmit={(e) => e.preventDefault()}>
      <Stack spacing={5}>
        <SettingSection>
          <Typography variant="h6">{t("settings.auditLog")}</Typography>
          <Typography variant="body2" color="text.secondary">
            {t("settings.auditLogDes")}
          </Typography>

          <SettingSection>
            <FormControlLabel
              control={
                <Switch
                  checked={enabled}
                  onChange={(e) =>
                    setSettings({
                      activity_log_enabled: e.target.checked ? "1" : "0",
                    })
                  }
                />
              }
              label={t("settings.activityLogEnabled")}
            />
            <NoMarginHelperText>{t("settings.activityLogEnabledDes")}</NoMarginHelperText>
          </SettingSection>

          <Divider sx={{ mt: 1 }} />

          <SettingSection>
            <Typography variant="subtitle1">{t("settings.activityLogCaptured")}</Typography>
            <Typography variant="body2" color="text.secondary">
              {t("settings.activityLogCapturedDes")}
            </Typography>
            <Stack spacing={1}>
              {activityCategories.map((category) => (
                <Box key={category}>
                  <Typography variant="body2">{t(category)}</Typography>
                  <Typography variant="caption" color="text.secondary">
                    {t(category + "Des")}
                  </Typography>
                </Box>
              ))}
            </Stack>
          </SettingSection>
        </SettingSection>
      </Stack>
    </Box>
  );
};

export default Events;
