import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Toaster } from "sonner";
import "@fontsource/instrument-serif/latin.css";
import "@fontsource/instrument-serif/latin-italic.css";
import "@fontsource-variable/archivo/wght.css";
import "@fontsource/archivo-black/latin.css";
import "@fontsource-variable/jetbrains-mono/wght.css";
import "lenis/dist/lenis.css";
import { AgentSurfaceProvider } from "./components/agent-surface/context";
import { ControlSessionProvider } from "./components/superhost/ControlSession";
import { ControlFrame } from "./components/superhost/ControlFrame";
import DifferenceCursor from "./components/difference-cursor";
import RequireRole from "./components/RequireRole";
import SmoothScroll from "./components/smooth-scroll";
import Debug from "./routes/debug";
import Dashboard from "./routes/dashboard";
import Documents from "./routes/documents";
import EntryRoute from "./routes/entry-route";
import Expansion from "./routes/expansion";
import Invoices from "./routes/invoices";
import Login from "./routes/login";
import Onboarding from "./routes/onboarding";
import Stay from "./routes/stay";
import CuratorJobs from "./routes/curator-jobs";
import CuratorJobDetail from "./routes/curator-job-detail";
import CuratorPropertyDetail from "./routes/curator-property-detail";
import CuratorZoneMap from "./routes/curator-zone-map";
import OpsCalendar from "./routes/ops-calendar";
import OpsProperties from "./routes/ops-properties";
import OpsTicketDetail from "./routes/ops-ticket-detail";
import OpsTicketNew from "./routes/ops-ticket-new";
import OpsTickets from "./routes/ops-tickets";
import OpsWorkers from "./routes/ops-workers";
import PackageShop from "./routes/package-shop";
import PropertyDetail from "./routes/property-detail";
import SessionExpiredModal from "./components/SessionExpiredModal";
import GlobalBackButton from "./components/GlobalBackButton";
import AgentCursor from "./components/superhost/AgentCursor";
import GlobalSuperhost from "./components/superhost/GlobalSuperhost";
import "./index.css";

const queryClient = new QueryClient({
  defaultOptions: { queries: { staleTime: 30_000, refetchOnWindowFocus: false } },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <SmoothScroll />
    <DifferenceCursor />
    <Toaster
      position="top-center"
      offset={104}
      visibleToasts={3}
      toastOptions={{
        unstyled: true,
        classNames: {
          toast: "cc-toast",
          title: "cc-toast-title",
          description: "cc-toast-description",
          error: "cc-toast-error",
          success: "cc-toast-success",
          closeButton: "cc-toast-close",
        },
      }}
    />
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <SessionExpiredModal />
        <GlobalBackButton />
        <AgentSurfaceProvider>
          <ControlSessionProvider>
          <Routes>
            <Route path="/" element={<EntryRoute />} />
            <Route path="/login" element={<Login />} />
            <Route path="/debug" element={<Debug />} />
            <Route path="/expansion" element={<Expansion />} />
            <Route path="/stay" element={<RequireRole allow={["guest"]}><Stay /></RequireRole>} />
            <Route path="/dashboard" element={<RequireRole allow={["owner"]}><Dashboard /></RequireRole>} />
            <Route path="/onboarding" element={<RequireRole allow={["owner"]}><Onboarding /></RequireRole>} />
            <Route path="/invoices" element={<RequireRole allow={["owner"]}><Invoices /></RequireRole>} />
            <Route path="/documents" element={<RequireRole allow={["owner"]}><Documents /></RequireRole>} />
            <Route path="/jobs" element={<RequireRole allow={["staff"]}><CuratorJobs /></RequireRole>} />
            <Route path="/jobs/map" element={<RequireRole allow={["staff"]}><CuratorZoneMap /></RequireRole>} />
            <Route path="/jobs/properties/:propertyId" element={<RequireRole allow={["staff"]}><CuratorPropertyDetail /></RequireRole>} />
            <Route path="/jobs/:ticketId" element={<RequireRole allow={["staff"]}><CuratorJobDetail /></RequireRole>} />
            <Route path="/properties/:propertyId" element={<RequireRole allow={["owner"]}><PropertyDetail /></RequireRole>} />
            <Route path="/properties/:propertyId/package" element={<RequireRole allow={["owner"]}><PackageShop /></RequireRole>} />
            <Route path="/ops/tickets" element={<RequireRole allow={["staff"]}><OpsTickets /></RequireRole>} />
            <Route path="/ops/tickets/new" element={<RequireRole allow={["staff"]}><OpsTicketNew /></RequireRole>} />
            <Route path="/ops/tickets/:ticketId" element={<RequireRole allow={["staff"]}><OpsTicketDetail /></RequireRole>} />
            <Route path="/ops/calendar" element={<RequireRole allow={["staff"]}><OpsCalendar /></RequireRole>} />
            <Route path="/ops/properties" element={<RequireRole allow={["staff"]}><OpsProperties /></RequireRole>} />
            <Route path="/ops/workers" element={<RequireRole allow={["staff"]}><OpsWorkers /></RequireRole>} />
            <Route path="*" element={<Navigate to="/login" replace />} />
          </Routes>
          <ControlFrame />
          <GlobalSuperhost />
          <AgentCursor />
          </ControlSessionProvider>
        </AgentSurfaceProvider>
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
);
