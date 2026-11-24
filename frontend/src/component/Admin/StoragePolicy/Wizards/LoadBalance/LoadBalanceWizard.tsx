import { Button } from "@mui/material";
import { useSnackbar } from "notistack";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { LoadBalanceStrategy, StoragePolicy } from "../../../../../api/dashboard";
import { PolicyType } from "../../../../../api/explorer";
import { DenseFilledTextField } from "../../../../Common/StyledComponents";
import { DefaultCloseAction } from "../../../../Common/Snackbar/snackbar";
import SettingForm from "../../../../Pages/Setting/SettingForm";
import { NoMarginHelperText } from "../../../Settings/Settings";
import { AddWizardProps } from "../../AddWizardDialog";
import LoadBalanceFields from "../../LoadBalanceFields";

const LoadBalanceWizard = ({ onSubmit }: AddWizardProps) => {
  const { t } = useTranslation("dashboard");
  const { enqueueSnackbar } = useSnackbar();
  const formRef = useRef<HTMLFormElement>(null);
  const [policy, setPolicy] = useState<StoragePolicy>({
    id: 0,
    name: "",
    type: PolicyType.load_balance,
    settings: {
      load_balance_strategy: LoadBalanceStrategy.round_robin,
      members: [],
    },
    edges: {},
  });

  const handleSubmit = () => {
    if (!formRef.current?.checkValidity()) {
      formRef.current?.reportValidity();
      return;
    }

    if (!policy.settings?.members?.length) {
      enqueueSnackbar(t("policy.loadBalanceMemberRequired"), {
        variant: "warning",
        action: DefaultCloseAction,
      });
      return;
    }

    onSubmit(policy);
  };

  return (
    <form ref={formRef} onSubmit={handleSubmit}>
      <SettingForm title={t("policy.name")} lgWidth={12}>
        <DenseFilledTextField
          fullWidth
          required
          value={policy.name}
          onChange={(e) => setPolicy({ ...policy, name: e.target.value })}
        />
        <NoMarginHelperText>{t("policy.policyName")}</NoMarginHelperText>
      </SettingForm>
      <LoadBalanceFields value={policy} onChange={(updater) => setPolicy(updater)} />
      <Button variant="contained" color="primary" sx={{ mt: 2 }} onClick={handleSubmit}>
        {t("policy.create")}
      </Button>
    </form>
  );
};

export default LoadBalanceWizard;
