import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const requiredKeys = [
  "fileManager.mountedPolicy",
  "fileManager.mountPolicy",
  "fileManager.currentStoragePolicy",
  "fileManager.inheritedFromParent",
  "fileManager.relocatePolicy",
  "fileManager.relocateConfirm",
  "fileManager.relocateNoFiles",
  "fileManager.relocateTaskCreated",
  "fileManager.relocateSourceTitle",
  "fileManager.relocateKeepSource",
  "fileManager.relocateKeepSourceDes",
  "fileManager.relocateDeleteSource",
  "fileManager.relocateDeleteSourceDes",
  "setting.relocateEntities",
  "setting.relocateFailed",
  "setting.relocateFailures",
];

const requiredDashboardKeys = [
  "policy.load_balance",
  "policy.childPolicy",
  "policy.selectPoliciesDes",
  "policy.loadBalanceNoMember",
  "policy.loadBalanceCompatibilityDes",
  "policy.loadBalanceStrategy",
  "policy.loadBalanceStrategyRoundRobin",
  "policy.loadBalanceStrategyDes",
  "policy.loadBalanceWeights",
  "policy.loadBalanceWeightDes",
  "policy.loadBalanceMemberRequired",
  "policy.xChildPolicies",
];

const locales = ["en-US", "it-IT"];

const checks = [
  { file: "application.json", keys: requiredKeys },
  { file: "dashboard.json", keys: requiredDashboardKeys },
];

const get = (obj, path) => path.split(".").reduce((acc, part) => (acc == null ? undefined : acc[part]), obj);

let failed = false;
let checked = 0;
for (const locale of locales) {
  for (const { file, keys } of checks) {
    const json = JSON.parse(readFileSync(resolve("public/locales", locale, file), "utf8"));
    for (const key of keys) {
      checked++;
      const value = get(json, key);
      if (typeof value !== "string" || value.length === 0) {
        console.error(`[i18n] missing key "${key}" in ${locale}/${file}`);
        failed = true;
      }
    }
  }
}

if (failed) {
  console.error("[i18n] build aborted: required translation keys are missing");
  process.exit(1);
}

console.log(`[i18n] ${checked} required keys present in ${locales.join(", ")}`);
