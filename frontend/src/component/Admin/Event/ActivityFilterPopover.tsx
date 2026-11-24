import { Box, Button, ListItemText, Popover, PopoverProps, Stack } from "@mui/material";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { DenseFilledTextField, DenseSelect } from "../../Common/StyledComponents";
import { SquareMenuItem } from "../../FileManager/ContextMenu/ContextMenu";
import SettingForm from "../../Pages/Setting/SettingForm";

export const activityActions = [
  "login_success",
  "login_failed",
  "file_create",
  "file_upload",
  "file_update",
  "file_rename",
  "file_move",
  "file_copy",
  "file_delete",
  "file_restore",
  "file_download",
  "share_create",
  "share_delete",
];

export interface ActivityFilterPopoverProps extends Omit<PopoverProps, "action"> {
  action: string;
  setAction: (value: string) => void;
  userID: string;
  setUserID: (value: string) => void;
  ip: string;
  setIP: (value: string) => void;
  from: string;
  setFrom: (value: string) => void;
  to: string;
  setTo: (value: string) => void;
  clearFilters: () => void;
}

const ActivityFilterPopover = ({
  action,
  setAction,
  userID,
  setUserID,
  ip,
  setIP,
  from,
  setFrom,
  to,
  setTo,
  clearFilters,
  onClose,
  open,
  ...rest
}: ActivityFilterPopoverProps) => {
  const { t } = useTranslation("dashboard");

  const [localAction, setLocalAction] = useState(action);
  const [localUserID, setLocalUserID] = useState(userID);
  const [localIP, setLocalIP] = useState(ip);
  const [localFrom, setLocalFrom] = useState(from);
  const [localTo, setLocalTo] = useState(to);

  useEffect(() => {
    if (open) {
      setLocalAction(action);
      setLocalUserID(userID);
      setLocalIP(ip);
      setLocalFrom(from);
      setLocalTo(to);
    }
  }, [open]);

  const handleApplyFilters = () => {
    setAction(localAction);
    setUserID(localUserID);
    setIP(localIP);
    setFrom(localFrom);
    setTo(localTo);
    onClose?.({}, "backdropClick");
  };

  const handleResetFilters = () => {
    setLocalAction("");
    setLocalUserID("");
    setLocalIP("");
    setLocalFrom("");
    setLocalTo("");
    clearFilters();
    onClose?.({}, "backdropClick");
  };

  return (
    <Popover
      anchorOrigin={{ vertical: "bottom", horizontal: "left" }}
      transformOrigin={{ vertical: "top", horizontal: "left" }}
      slotProps={{ paper: { sx: { p: 2, width: 320, maxWidth: "100%" } } }}
      onClose={onClose}
      open={open}
      {...rest}
    >
      <Stack spacing={2}>
        <SettingForm title={t("activity.action")} noContainer lgWidth={12}>
          <DenseSelect
            fullWidth
            displayEmpty
            value={localAction != "" ? localAction : " "}
            onChange={(e) => setLocalAction(e.target.value === " " ? "" : (e.target.value as string))}
          >
            {activityActions.map((value) => (
              <SquareMenuItem key={value} value={value}>
                <ListItemText
                  primary={t(`common:activity.action.${value}`, value)}
                  slotProps={{ primary: { variant: "body2" } }}
                />
              </SquareMenuItem>
            ))}
            <SquareMenuItem value={" "}>
              <ListItemText primary={<em>{t("user.all")}</em>} slotProps={{ primary: { variant: "body2" } }} />
            </SquareMenuItem>
          </DenseSelect>
        </SettingForm>

        <SettingForm title={t("event.userID")} noContainer lgWidth={12}>
          <DenseFilledTextField
            fullWidth
            value={localUserID}
            onChange={(e) => setLocalUserID(e.target.value)}
            placeholder={t("user.emptyNoFilter")}
            size="small"
          />
        </SettingForm>

        <SettingForm title={t("event.ip")} noContainer lgWidth={12}>
          <DenseFilledTextField
            fullWidth
            value={localIP}
            onChange={(e) => setLocalIP(e.target.value)}
            placeholder={t("user.emptyNoFilter")}
            size="small"
          />
        </SettingForm>

        <SettingForm title={t("activity.from")} noContainer lgWidth={12}>
          <DenseFilledTextField
            fullWidth
            type="date"
            value={localFrom}
            onChange={(e) => setLocalFrom(e.target.value)}
            size="small"
            slotProps={{ inputLabel: { shrink: true } }}
          />
        </SettingForm>

        <SettingForm title={t("activity.to")} noContainer lgWidth={12}>
          <DenseFilledTextField
            fullWidth
            type="date"
            value={localTo}
            onChange={(e) => setLocalTo(e.target.value)}
            size="small"
            slotProps={{ inputLabel: { shrink: true } }}
          />
        </SettingForm>

        <Box display="flex" justifyContent="space-between">
          <Button variant="outlined" size="small" onClick={handleResetFilters}>
            {t("user.reset")}
          </Button>
          <Button variant="contained" size="small" onClick={handleApplyFilters}>
            {t("user.apply")}
          </Button>
        </Box>
      </Stack>
    </Popover>
  );
};

export default ActivityFilterPopover;
