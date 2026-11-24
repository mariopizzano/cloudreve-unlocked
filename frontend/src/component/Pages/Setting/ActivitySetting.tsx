import {
  Box,
  Chip,
  Divider,
  List,
  ListItem,
  ListItemText,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Typography,
} from "@mui/material";
import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { getUserActivityList } from "../../../api/api";
import { ActivityEntry, ListActivityResponse } from "../../../api/dashboard";
import { useAppDispatch } from "../../../redux/hooks";
import { formatLocalTime } from "../../../util/datetime";
import TablePagination from "../../Admin/Common/TablePagination";
import { NoWrapTableCell, StyledTableContainerPaper } from "../../Common/StyledComponents";

const pageSize = 10;

const toDate = (value: string) => formatLocalTime(value);

const ActivitySetting = () => {
  const { t } = useTranslation("application");
  const dispatch = useAppDispatch();

  const [filePage, setFilePage] = useState(1);
  const [loginPage, setLoginPage] = useState(1);
  const [fileResult, setFileResult] = useState<ListActivityResponse>();
  const [loginResult, setLoginResult] = useState<ListActivityResponse>();
  const [fileLoading, setFileLoading] = useState(true);
  const [loginLoading, setLoginLoading] = useState(true);

  const loadFileActivity = useCallback(() => {
    setFileLoading(true);
    dispatch(getUserActivityList({ page: filePage, page_size: pageSize, scope: "file" }))
      .then((res) => setFileResult(res))
      .finally(() => setFileLoading(false));
  }, [dispatch, filePage]);

  const loadLoginActivity = useCallback(() => {
    setLoginLoading(true);
    dispatch(getUserActivityList({ page: loginPage, page_size: pageSize, scope: "login" }))
      .then((res) => setLoginResult(res))
      .finally(() => setLoginLoading(false));
  }, [dispatch, loginPage]);

  useEffect(() => {
    loadFileActivity();
  }, [loadFileActivity]);

  useEffect(() => {
    loadLoginActivity();
  }, [loadLoginActivity]);

  const fileActivities: ActivityEntry[] = fileResult?.activities ?? [];
  const loginActivities: ActivityEntry[] = loginResult?.activities ?? [];

  return (
    <Stack spacing={4}>
      <Box>
        <Typography variant="h6" gutterBottom>
          {t("activity.fileHistory")}
        </Typography>
        <TableContainer component={StyledTableContainerPaper}>
          <Table size="small" sx={{ tableLayout: "fixed" }}>
            <TableHead>
              <TableRow>
                <NoWrapTableCell width={180}>{t("common:activity.time")}</NoWrapTableCell>
                <NoWrapTableCell width={180}>{t("common:activity.actionLabel")}</NoWrapTableCell>
                <NoWrapTableCell>{t("common:activity.target")}</NoWrapTableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {!fileLoading &&
                fileActivities.map((entry) => (
                  <TableRow key={entry.id} hover>
                    <NoWrapTableCell>{toDate(entry.created_at)}</NoWrapTableCell>
                    <NoWrapTableCell>
                      {t(`common:activity.action.${entry.action}`, entry.action)}
                    </NoWrapTableCell>
                    <NoWrapTableCell>
                      <Box
                        sx={{
                          overflow: "hidden",
                          textOverflow: "ellipsis",
                          whiteSpace: "nowrap",
                        }}
                        title={entry.target}
                      >
                        {entry.target || "-"}
                      </Box>
                    </NoWrapTableCell>
                  </TableRow>
                ))}
              {!fileLoading && fileActivities.length === 0 && (
                <TableRow>
                  <TableCell colSpan={3} align="center">
                    {t("common:activity.noActivity")}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </TableContainer>
        {(fileResult?.pagination?.total_items ?? 0) > 0 && (
          <TablePagination
            page={filePage}
            totalItems={fileResult?.pagination?.total_items ?? 0}
            rowsPerPage={pageSize}
            rowsPerPageOptions={[10]}
            onChange={(_, value) => setFilePage(value)}
          />
        )}
      </Box>

      <Divider />

      <Box>
        <Typography variant="h6" gutterBottom>
          {t("activity.recentLogins")}
        </Typography>
        <List disablePadding>
          {!loginLoading &&
            loginActivities.map((entry) => (
              <ListItem key={entry.id} divider disableGutters>
                <ListItemText
                  primary={
                    <Stack direction="row" spacing={1} alignItems="center">
                      <Chip
                        size="small"
                        color={entry.action === "login_failed" ? "error" : "success"}
                        label={t(`common:activity.action.${entry.action}`, entry.action)}
                      />
                      <Typography variant="body2" color="text.secondary">
                        {toDate(entry.created_at)}
                      </Typography>
                    </Stack>
                  }
                  secondary={
                    <Typography variant="caption" color="text.secondary" component="span">
                      {[entry.ip, entry.user_agent].filter(Boolean).join(" · ")}
                    </Typography>
                  }
                />
              </ListItem>
            ))}
          {!loginLoading && loginActivities.length === 0 && (
            <Typography variant="body2" color="text.secondary" sx={{ py: 2 }}>
              {t("common:activity.noActivity")}
            </Typography>
          )}
        </List>
        {(loginResult?.pagination?.total_items ?? 0) > 0 && (
          <TablePagination
            page={loginPage}
            totalItems={loginResult?.pagination?.total_items ?? 0}
            rowsPerPage={pageSize}
            rowsPerPageOptions={[10]}
            onChange={(_, value) => setLoginPage(value)}
          />
        )}
      </Box>
    </Stack>
  );
};

export default ActivitySetting;
