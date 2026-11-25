import { defineConfig } from "mygo-cli";
import pkg from "./package.json" with { type: "json" };

export default defineConfig({
  name: "Eagle",
  identifier: "com.example.eagle",
  // The release workflow checks that a tag such as v0.2.0 matches it.
  version: pkg.version,
  // `mygo dev` runs devCommand and loads devUrl; `mygo build` runs
  // buildCommand and embeds frontendDist into the app.
  devUrl: "http://localhost:5173",
  devCommand: "bun run dev:web",
  buildCommand: "bun run build:web",
  frontendDist: "dist",
  bindings: "src/mygo.ts",
  out: "build",
  // Installed apps update from the releases of the repository, signed with
  // the key whose public half is here (mygo build reads the secret one from
  // MYGO_UPDATER_PRIVATE_KEY). Release notes come from CHANGELOG.md.
  updates: {
    publicKey: "7JV1L2sp1RmQr2p98h/z+YOs+CoS4LkoKBhqbasZUug=",
    github: "limboy/eagle",
  },
  macos: {
    signingIdentity: "Developer ID Application: LI ZHONG (5P9ZHW7578)",
    // The notary profile is in the runner's keychain in CI (see
    // .github/workflows/release.yml), and in the login keychain here.
    notarize: { keychainProfile: "notary", keychain: process.env.NOTARY_KEYCHAIN },
  },
  // Phones: `mygo dev -platform ios` and `-platform android` run the app on a
  // simulator, an emulator or a device (`mygo devices` lists them, and
  // `mygo doctor` tells what is missing). What devices and stores need:
  // ios: { team: "ABCDE12345", infoPlist: { NSCameraUsageDescription: "Scan receipts." } },
  // android: { keystore: "upload.jks", keyAlias: "upload", permissions: ["CAMERA"] },
});
