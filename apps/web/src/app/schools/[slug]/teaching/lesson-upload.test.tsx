/** @vitest-environment jsdom */

import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import LessonUpload from "./lesson-upload";

const origin = "https://objects.example.test";
const endpoint = "/api/v1/tenants/fictional/teaching/lessons/fixture/media";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

async function upload() {
  const user = userEvent.setup();
  const attached = vi.fn();
  render(<LessonUpload endpoint={endpoint} csrf="fixture-csrf" origin={origin}
    disabled={false} onAttached={attached} onBusy={() => {}} />);
  const file = new File(["%PDF-1.7"], "fixture.pdf", { type: "application/pdf" });
  Object.defineProperty(file, "arrayBuffer", { value: async () => new ArrayBuffer(file.size) });
  vi.stubGlobal("crypto", { subtle: { digest: vi.fn().mockResolvedValue(new ArrayBuffer(32)) } });
  await user.upload(screen.getByLabelText("Lesson attachment"), file);
  fireEvent.submit(screen.getByRole("button", { name: "Upload attachment" }).closest("form")!);
  return attached;
}

it("uploads without app credentials and confirms scanning before reporting success", async () => {
  const fetch = vi.fn()
    .mockResolvedValueOnce(Response.json({ data: { id: "fixture-id", upload: { url: origin + "/quarantine/fixture" } } }))
    .mockResolvedValueOnce(new Response(null, { status: 200 }))
    .mockResolvedValueOnce(Response.json({ data: { status: "READY" } }));
  vi.stubGlobal("fetch", fetch);
  const attached = await upload();

  expect((await screen.findByRole("status")).textContent).toContain("scanned and added");
  expect(fetch).toHaveBeenNthCalledWith(1, endpoint, expect.objectContaining({
    credentials: "same-origin", headers: expect.objectContaining({ "X-CSRF-Token": "fixture-csrf" }),
  }));
  expect(fetch).toHaveBeenNthCalledWith(2, new URL(origin + "/quarantine/fixture"), expect.objectContaining({
    method: "PUT", credentials: "omit", redirect: "error", referrerPolicy: "no-referrer",
  }));
  expect(new Headers(fetch.mock.calls[1][1].headers).has("X-CSRF-Token")).toBe(false);
  expect(fetch).toHaveBeenNthCalledWith(3, endpoint + "/fixture-id/complete", expect.objectContaining({ method: "POST" }));
  expect(attached).toHaveBeenCalledOnce();
});

it("never sends the file to a different origin", async () => {
  const fetch = vi.fn().mockResolvedValue(Response.json({
    data: { id: "fixture-id", upload: { url: "https://other.example.test/quarantine/fixture" } },
  }));
  vi.stubGlobal("fetch", fetch);
  const attached = await upload();

  expect((await screen.findByRole("alert")).textContent).toBe("The upload could not be confirmed. Reload the draft before trying again.");
  expect(fetch).toHaveBeenCalledOnce();
  expect(attached).not.toHaveBeenCalled();
});

it("shows a safe error when validation or scanning rejects the upload", async () => {
  const fetch = vi.fn()
    .mockResolvedValueOnce(Response.json({ data: { id: "fixture-id", upload: { url: origin + "/quarantine/fixture" } } }))
    .mockResolvedValueOnce(new Response(null, { status: 200 }))
    .mockResolvedValueOnce(new Response("internal fixture diagnostic", { status: 422 }));
  vi.stubGlobal("fetch", fetch);
  const attached = await upload();

  expect((await screen.findByRole("alert")).textContent).toBe("The attachment did not pass file validation or scanning.");
  expect(attached).not.toHaveBeenCalled();
});
