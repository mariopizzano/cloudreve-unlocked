import {
  Badge,
  Box,
  Container,
  Stack,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Tooltip,
} from "@mui/material";
import { bindPopover, bindTrigger, usePopupState } from "material-ui-popup-state/hooks";
import { useQueryState } from "nuqs";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { getActivityList } from "../../../api/api";
import { ActivityEntry, ActivityListQuery } from "../../../api/dashboard";
import { useAppDispatch } from "../../../redux/hooks";
import {
  NoWrapTableCell,
  SecondaryButton,
  StyledTableContainerPaper,
} from "../../Common/StyledComponents";
import ArrowSync from "../../Icons/ArrowSync";
import Filter from "../../Icons/Filter";
import PageContainer from "../../Pages/PageContainer";
import PageHeader from "../../Pages/PageHeader";
import { formatLocalTime } from "../../../util/datetime";
import TablePagination from "../Common/TablePagination";
import { OrderByQuery, PageQuery, PageSizeQuery } from "../StoragePolicy/StoragePolicySetting";
import UserDialog from "../User/UserDialog/UserDialog";
import ActivityDialog from "./ActivityDialog";
import ActivityFilterPopover from "./ActivityFilterPopover";

export const ActionQuery = "action";
export const UserQuery = "user_id";
export const IPQuery = "ip";
export const FromQuery = "from";
export const ToQuery = "to";

const ActivityList = () => {
  const { t } = useTranslation("dashboard");
  const dispatch = useAppDispatch();
  const [loading, setLoading] = useState(true);
  const [activities, setActivities] = useState<ActivityEntry[]>([]);
  const [count, setCount] = useState(0);
  const [page, setPage] = useQueryState(PageQuery, { defaultValue: "1" });
  const [pageSize, setPageSize] = useQueryState(PageSizeQuery, { defaultValue: "20" });
  const [action, setAction] = useQueryState(ActionQuery, { defaultValue: "" });
  const [userID, setUserID] = useQueryState(UserQuery, { defaultValue: "" });
  const [ip, setIP] = useQueryState(IPQuery, { defaultValue: "" });
  const [from, setFrom] = useQueryState(FromQuery, { defaultValue: "" });
  const [to, setTo] = useQueryState(ToQuery, { defaultValue: "" });
  const [orderBy] = useQueryState(OrderByQuery, { defaultValue: "" });
  const [openActivity, setOpenActivity] = useState<ActivityEntry | undefined>();
  const [userDialogOpen, setUserDialogOpen] = useState(false);
  const [userDialogID, setUserDialogID] = useState<number | undefined>();

  const filterPopupState = usePopupState({ variant: "popover", popupId: "activityFilterPopover" });

  const pageInt = parseInt(page ?? "1") || 1;
  const pageSizeInt = parseInt(pageSize ?? "20") || 20;

  const fetchActivities = useCallback(() => {
    setLoading(true);
    const params: ActivityListQuery = {
      page: pageInt,
      page_size: pageSizeInt,
    };
    if (action) params.action = action;
    if (userID) params.user_id = parseInt(userID) || undefined;
    if (ip) params.ip = ip;
    if (from) params.from = from;
    if (to) params.to = to;

    dispatch(getActivityList(params))
      .then((res) => {
        setActivities(res.activities ?? []);
        setCount(res.pagination?.total_items ?? 0);
      })
      .finally(() => setLoading(false));
  }, [dispatch, pageInt, pageSizeInt, action, userID, ip, from, to, orderBy]);

  useEffect(() => {
    fetchActivities();
  }, [fetchActivities]);

  const clearFilters = useCallback(() => {
    setAction("");
    setUserID("");
    setIP("");
    setFrom("");
    setTo("");
  }, [setAction, setUserID, setIP, setFrom, setTo]);

  const hasActiveFilters = useMemo(() => {
    return !!(action || userID || ip || from || to);
  }, [action, userID, ip, from, to]);

  const handleOpenUser = (id?: number) => {
    if (!id) return;
    setUserDialogID(id);
    setUserDialogOpen(true);
  };

  return (
    <PageContainer>
      <UserDialog open={userDialogOpen} onClose={() => setUserDialogOpen(false)} userID={userDialogID} />
      <ActivityDialog
        open={!!openActivity}
        onClose={() => setOpenActivity(undefined)}
        activity={openActivity}
      />
      <Container maxWidth="xl">
        <PageHeader title={t("dashboard:nav.events")} />
        <Stack direction="row" spacing={1} sx={{ mb: 2 }}>
          <ActivityFilterPopover
            {...bindPopover(filterPopupState)}
            action={action ?? ""}
            setAction={setAction}
            userID={userID ?? ""}
            setUserID={setUserID}
            ip={ip ?? ""}
            setIP={setIP}
            from={from ?? ""}
            setFrom={setFrom}
            to={to ?? ""}
            setTo={setTo}
            clearFilters={clearFilters}
          />

          <SecondaryButton
            onClick={fetchActivities}
            disabled={loading}
            variant={"contained"}
            startIcon={<ArrowSync />}
          >
            {t("node.refresh")}
          </SecondaryButton>

          <Badge color="primary" variant="dot" invisible={!hasActiveFilters}>
            <SecondaryButton startIcon={<Filter />} variant="contained" {...bindTrigger(filterPopupState)}>
              {t("user.filter")}
            </SecondaryButton>
          </Badge>
        </Stack>

        <TableContainer component={StyledTableContainerPaper} sx={{ mt: 2 }}>
          <Table size="small" stickyHeader sx={{ width: "100%", tableLayout: "fixed" }}>
            <TableHead>
              <TableRow>
                <NoWrapTableCell width={190}>{t("event.datetime")}</NoWrapTableCell>
                <NoWrapTableCell width={110}>{t("event.userID")}</NoWrapTableCell>
                <NoWrapTableCell width={170}>{t("activity.action")}</NoWrapTableCell>
                <NoWrapTableCell>{t("common:activity.target")}</NoWrapTableCell>
                <NoWrapTableCell width={140}>{t("event.ip")}</NoWrapTableCell>
                <NoWrapTableCell width={220}>{t("event.userAgent")}</NoWrapTableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {!loading &&
                activities.map((entry) => (
                  <TableRow hover key={entry.id} sx={{ cursor: "pointer" }} onClick={() => setOpenActivity(entry)}>
                    <NoWrapTableCell>{formatLocalTime(entry.created_at)}</NoWrapTableCell>
                    <NoWrapTableCell
                      onClick={(e) => {
                        e.stopPropagation();
                        handleOpenUser(entry.user_id);
                      }}
                      sx={{ cursor: entry.user_id ? "pointer" : "default", color: entry.user_id ? "primary.main" : undefined }}
                    >
                      {entry.user_id ?? "-"}
                    </NoWrapTableCell>
                    <NoWrapTableCell>{t(`common:activity.action.${entry.action}`, entry.action)}</NoWrapTableCell>
                    <NoWrapTableCell>
                      <Tooltip title={entry.target ?? ""} placement="top">
                        <Box
                          sx={{
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            whiteSpace: "nowrap",
                          }}
                        >
                          {entry.target || "-"}
                        </Box>
                      </Tooltip>
                    </NoWrapTableCell>
                    <NoWrapTableCell>{entry.ip || "-"}</NoWrapTableCell>
                    <NoWrapTableCell>
                      <Tooltip title={entry.user_agent ?? ""} placement="top">
                        <Box
                          sx={{
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                            whiteSpace: "nowrap",
                          }}
                        >
                          {entry.user_agent || "-"}
                        </Box>
                      </Tooltip>
                    </NoWrapTableCell>
                  </TableRow>
                ))}
              {loading &&
                Array.from(Array(5)).map((_, index) => (
                  <TableRow key={`loading-${index}`}>
                    {Array.from(Array(6)).map((__, cell) => (
                      <TableCell key={cell}>
                        <Box sx={{ height: 20 }} />
                      </TableCell>
                    ))}
                  </TableRow>
                ))}
              {!loading && activities.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6} align="center">
                    {t("application:setting.listEmpty")}
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </TableContainer>

        {count > 0 && (
          <Box sx={{ mt: 1 }}>
            <TablePagination
              page={pageInt}
              totalItems={count}
              rowsPerPage={pageSizeInt}
              rowsPerPageOptions={[10, 20, 50, 100]}
              onRowsPerPageChange={(value) => {
                setPageSize(value.toString());
                setPage("1");
              }}
              onChange={(_, value) => setPage(value.toString())}
            />
          </Box>
        )}
      </Container>
    </PageContainer>
  );
};

export default ActivityList;
