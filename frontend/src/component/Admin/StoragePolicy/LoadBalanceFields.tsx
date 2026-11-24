import { Box, Checkbox, FormControl, ListItemText, Typography } from "@mui/material";
import { SelectChangeEvent } from "@mui/material/Select";
import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { getStoragePolicyList } from "../../../api/api";
import { LoadBalanceMember, LoadBalanceStrategy, StoragePolicy } from "../../../api/dashboard";
import { PolicyType } from "../../../api/explorer";
import { useAppDispatch } from "../../../redux/hooks";
import FacebookCircularProgress from "../../Common/CircularProgress";
import { DenseFilledTextField, DenseSelect } from "../../Common/StyledComponents";
import { SquareMenuItem } from "../../FileManager/ContextMenu/ContextMenu";
import SettingForm from "../../Pages/Setting/SettingForm";
import { NoMarginHelperText } from "../Settings/Settings";

export interface LoadBalanceFieldsProps {
  value: StoragePolicy;
  onChange: (updater: (policy: StoragePolicy) => StoragePolicy) => void;
}

const maxWeight = 1000;

const normalizeWeight = (weight: number): number => {
  if (!Number.isFinite(weight) || weight < 1) {
    return 1;
  }
  return Math.min(maxWeight, Math.floor(weight));
};

// LoadBalanceFields edits the members, their weights and the strategy of a load_balance
// policy. It is shared between the creation wizard and the policy editor.
const LoadBalanceFields = ({ value, onChange }: LoadBalanceFieldsProps) => {
  const { t } = useTranslation("dashboard");
  const dispatch = useAppDispatch();
  const [policies, setPolicies] = useState<StoragePolicy[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    setLoading(true);
    dispatch(getStoragePolicyList({ page: 1, page_size: 1000, order_by: "id", order_direction: "asc" }))
      .then((res) => {
        // A load balance policy cannot include another load balance policy.
        setPolicies(res.policies.filter((policy) => policy.type !== PolicyType.load_balance));
      })
      .finally(() => {
        setLoading(false);
      });
  }, [dispatch]);

  const members = useMemo(() => value.settings?.members ?? [], [value.settings?.members]);
  const selected = useMemo(() => members.map((member) => member.policy_id), [members]);
  const policyMap = useMemo(() => {
    return policies.reduce(
      (acc, policy) => {
        acc[policy.id] = policy;
        return acc;
      },
      {} as Record<number, StoragePolicy>,
    );
  }, [policies]);

  const onMembersChange = (event: SelectChangeEvent<unknown>) => {
    const ids = event.target.value as number[];
    onChange((policy) => {
      const current = policy.settings?.members ?? [];
      const next: LoadBalanceMember[] = ids.map((id) => {
        const existing = current.find((member) => member.policy_id === id);
        return existing ?? { policy_id: id, weight: 1 };
      });
      return {
        ...policy,
        settings: { ...policy.settings, members: next },
      };
    });
  };

  const onWeightChange = (policyID: number, weight: number) => {
    onChange((policy) => ({
      ...policy,
      settings: {
        ...policy.settings,
        members: (policy.settings?.members ?? []).map((member) =>
          member.policy_id === policyID ? { ...member, weight: normalizeWeight(weight) } : member,
        ),
      },
    }));
  };

  return (
    <>
      <SettingForm title={t("policy.childPolicy")} lgWidth={5}>
        <FormControl fullWidth>
          <DenseSelect
            multiple
            value={selected}
            onChange={onMembersChange}
            disabled={loading}
            required
            renderValue={(value) => {
              const selectedIDs = value as number[];
              return (
                <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5 }}>
                  {selectedIDs.length === 0 ? (
                    <Typography variant="body2" color="text.secondary">
                      {t("policy.loadBalanceNoMember")}
                    </Typography>
                  ) : (
                    selectedIDs.map((id) => (
                      <Typography key={id} variant="body2">
                        {policyMap[id]?.name ?? id}
                        {id !== selectedIDs[selectedIDs.length - 1] ? ", " : ""}
                      </Typography>
                    ))
                  )}
                </Box>
              );
            }}
            MenuProps={{
              PaperProps: { sx: { maxHeight: 360 } },
            }}
          >
            {loading && (
              <SquareMenuItem disabled value={-1}>
                <FacebookCircularProgress size={20} />
              </SquareMenuItem>
            )}
            {!loading &&
              policies.map((policy) => (
                <SquareMenuItem key={policy.id} value={policy.id}>
                  <Checkbox size="small" checked={selected.indexOf(policy.id) > -1} tabIndex={-1} disableRipple />
                  <ListItemText
                    slotProps={{
                      primary: { variant: "body2" },
                      secondary: { variant: "caption" },
                    }}
                    primary={policy.name}
                    secondary={t(`policy.${policy.type}`)}
                  />
                </SquareMenuItem>
              ))}
          </DenseSelect>
          <NoMarginHelperText>{t("policy.selectPoliciesDes")}</NoMarginHelperText>
          <NoMarginHelperText>{t("policy.loadBalanceCompatibilityDes")}</NoMarginHelperText>
        </FormControl>
      </SettingForm>
      {members.length > 0 && (
        <SettingForm title={t("policy.loadBalanceWeights")} lgWidth={5}>
          <FormControl fullWidth>
            <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
              {members.map((member) => (
                <Box key={member.policy_id} sx={{ display: "flex", alignItems: "center", gap: 1 }}>
                  <Typography variant="body2" sx={{ flexGrow: 1 }}>
                    {policyMap[member.policy_id]?.name ?? member.policy_id}
                  </Typography>
                  <DenseFilledTextField
                    type="number"
                    value={member.weight}
                    onChange={(e) => onWeightChange(member.policy_id, parseInt(e.target.value))}
                    slotProps={{ htmlInput: { min: 1, max: maxWeight } }}
                    sx={{ width: 120 }}
                  />
                </Box>
              ))}
            </Box>
            <NoMarginHelperText>{t("policy.loadBalanceWeightDes")}</NoMarginHelperText>
          </FormControl>
        </SettingForm>
      )}
      <SettingForm title={t("policy.loadBalanceStrategy")} lgWidth={5}>
        <FormControl fullWidth>
          <DenseSelect
            value={value.settings?.load_balance_strategy ?? LoadBalanceStrategy.round_robin}
            onChange={(event) =>
              onChange((policy) => ({
                ...policy,
                settings: {
                  ...policy.settings,
                  load_balance_strategy: event.target.value as LoadBalanceStrategy,
                },
              }))
            }
          >
            <SquareMenuItem value={LoadBalanceStrategy.round_robin}>
              <ListItemText slotProps={{ primary: { variant: "body2" } }}>
                {t("policy.loadBalanceStrategyRoundRobin")}
              </ListItemText>
            </SquareMenuItem>
          </DenseSelect>
          <NoMarginHelperText>{t("policy.loadBalanceStrategyDes")}</NoMarginHelperText>
        </FormControl>
      </SettingForm>
    </>
  );
};

export default LoadBalanceFields;
