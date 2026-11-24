import {
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  Stack,
  Typography,
} from "@mui/material";
import { useTranslation } from "react-i18next";
import { ActivityEntry } from "../../../api/dashboard";
import { formatLocalTime } from "../../../util/datetime";

export interface ActivityDialogProps {
  open: boolean;
  onClose: () => void;
  activity?: ActivityEntry;
}

const ActivityRow = ({ label, value }: { label: string; value?: string | number }) => {
  if (value === undefined || value === null || value === "") {
    return null;
  }

  return (
    <Stack direction={{ xs: "column", sm: "row" }} spacing={1}>
      <Typography variant="body2" color="text.secondary" sx={{ minWidth: 140 }}>
        {label}
      </Typography>
      <Typography variant="body2" sx={{ wordBreak: "break-all" }}>
        {value}
      </Typography>
    </Stack>
  );
};

const ActivityDialog = ({ open, onClose, activity }: ActivityDialogProps) => {
  const { t } = useTranslation("dashboard");

  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle>{t("activity.dialogTitle")}</DialogTitle>
      <DialogContent dividers>
        {activity && (
          <Stack spacing={1.5}>
            <ActivityRow
              label={t("activity.action")}
              value={t(`common:activity.action.${activity.action}`, activity.action)}
            />
            <ActivityRow label={t("event.datetime")} value={formatLocalTime(activity.created_at)} />
            <ActivityRow label={t("event.userID")} value={activity.user_id} />
            <ActivityRow label={t("common:activity.target")} value={activity.target} />
            <ActivityRow label={t("event.fileID")} value={activity.file_id} />
            <ActivityRow label={t("event.ip")} value={activity.ip} />
            <ActivityRow label={t("event.userAgent")} value={activity.user_agent} />
            <ActivityRow label={t("event.type")} value={activity.target_type} />
            {activity.metadata && Object.keys(activity.metadata).length > 0 && (
              <>
                <Divider />
                <Typography variant="subtitle2">{t("event.rawContent")}</Typography>
                <Box
                  component="pre"
                  sx={{
                    m: 0,
                    p: 1.5,
                    borderRadius: 1,
                    backgroundColor: (theme) => theme.palette.action.hover,
                    fontSize: "0.8rem",
                    overflow: "auto",
                    maxHeight: 320,
                  }}
                >
                  {JSON.stringify(activity.metadata, null, 2)}
                </Box>
              </>
            )}
          </Stack>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{t("common:close")}</Button>
      </DialogActions>
    </Dialog>
  );
};

export default ActivityDialog;
