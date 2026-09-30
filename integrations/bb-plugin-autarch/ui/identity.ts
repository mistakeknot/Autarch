// The source identity this app bundle was built with. scripts/build-identity.mjs stamps
// identity.json in its export; the Home root carries it as data-home-source so the
// scenario harness can prove which frontend is loaded [G-7].
import identity from "../identity.json";

export const HOME_SOURCE: string = identity.source_sha256;
