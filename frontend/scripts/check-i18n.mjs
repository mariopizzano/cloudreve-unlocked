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
];

const locales = ["en-US", "it-IT"];

const get = (obj, path) => path.split(".").reduce((acc, part) => (acc == null ? undefined : acc[part]), obj);

let failed = false;
for (const locale of locales) {
  const file = resolve("public/locales", locale, "application.json");
  const json = JSON.parse(readFileSync(file, "utf8"));
  for (const key of requiredKeys) {
    const value = get(json, key);
    if (typeof value !== "string" || value.length === 0) {
      console.error(`[i18n] missing key "${key}" in ${locale}/application.json`);
      failed = true;
    }
  }
}

if (failed) {
  console.error("[i18n] build aborted: required translation keys are missing");
  process.exit(1);
}

console.log(`[i18n] ${requiredKeys.length} required keys present in ${locales.join(", ")}`);
