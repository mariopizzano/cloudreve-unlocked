import { Typography } from "@mui/material";
import { useContext } from "react";
import { useTranslation } from "react-i18next";
import { SettingSection, SettingSectionContent } from "../../../Settings/Settings";
import LoadBalanceFields from "../../LoadBalanceFields";
import { StoragePolicySettingContext } from "../StoragePolicySettingWrapper";

const LoadBalanceSection = () => {
  const { t } = useTranslation("dashboard");
  const { values, setPolicy } = useContext(StoragePolicySettingContext);

  return (
    <SettingSection>
      <Typography variant="h6" gutterBottom>
        {t("policy.load_balance")}
      </Typography>
      <SettingSectionContent>
        <LoadBalanceFields value={values} onChange={setPolicy} />
      </SettingSectionContent>
    </SettingSection>
  );
};

export default LoadBalanceSection;
