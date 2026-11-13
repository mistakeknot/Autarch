// @vitest-environment jsdom
import { act } from "react";
import { createRoot } from "react-dom/client";
import { expect, it, vi } from "vitest";
const { call } = vi.hoisted(() => ({call: vi.fn()}));
vi.mock("@get-bb/plugin-sdk/app", () => ({useRpc: () => rpc}));
const rpc={call};
import { EvidenceBlock } from "../ui/evidence.js";
Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
it("loads local thumbnails by hash, opens full size and releases the object URL", async () => {
 const create=vi.fn(()=>"blob:local-image"), revoke=vi.fn();
 Object.defineProperty(URL,"createObjectURL",{value:create,configurable:true}); Object.defineProperty(URL,"revokeObjectURL",{value:revoke,configurable:true});
 call.mockResolvedValue({ok:true,mime:"image/png",data_base64:"iVBORw0KGgo="});
 const div=document.createElement("div"), root=createRoot(div);
 try {
  await act(async()=>{root.render(<EvidenceBlock items={[{caption:"Screenshot",image:{sha256:"a".repeat(64),mime:"image/png"}}]} />)});
  expect(call).toHaveBeenCalledWith("evidenceImage",{sha256:"a".repeat(64)});
  expect(div.querySelector("img")?.getAttribute("src")).toBe("blob:local-image");
  expect(div.querySelector("a")?.getAttribute("href")).toBe("blob:local-image");
  expect(div.querySelector("a")?.getAttribute("rel")).toBe("noopener noreferrer");
 } finally { await act(async()=>root.unmount()); }
 expect(revoke).toHaveBeenCalledWith("blob:local-image");
});
it("keeps missing images quiet and never uses a remote image URL", async () => {
 call.mockResolvedValue({ok:false}); const div=document.createElement("div"),root=createRoot(div);
 try { await act(async()=>{root.render(<EvidenceBlock items={[{caption:"Missing",image:{sha256:"b".repeat(64),mime:"image/png"}}]} />)}); expect(div.textContent).toContain("image unavailable"); expect(div.querySelector("img")).toBeNull(); }
 finally { await act(async()=>root.unmount()); }
});
