import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { RequireAuth } from "./auth/PolicyGuard";
import { useAuthStore } from "./auth/store";
import { useLocaleStore } from "./i18n/locale";
import { AppShell } from "./layout/AppShell";
import { ApprovalsPage } from "./pages/ApprovalsPage";
import { DashboardPage } from "./pages/DashboardPage";
import { InventoryPage } from "./pages/InventoryPage";
import { LoginPage } from "./pages/LoginPage";
import { PayrollPage } from "./pages/PayrollPage";
import { ImageReportsPage } from "./pages/ImageReportsPage";
import { MobileUploadPage } from "./pages/MobileUploadPage";
import { ReceivingPage } from "./pages/ReceivingPage";
import { ReportsPage } from "./pages/ReportsPage";
import { SearchPage } from "./pages/SearchPage";
import { SetupPage, SetupRedirect } from "./pages/SetupPage";
import { ShippingSlipsPage } from "./pages/ShippingSlipsPage";
import { TransportSheetsPage } from "./pages/TransportSheetsPage";
import { MailPage } from "./pages/MailPage";
import { TransfersPage } from "./pages/TransfersPage";
import { ParcelsPage } from "./pages/ParcelsPage";
import { PublicStorefrontPage } from "./pages/PublicStorefrontPage";
import { CustomerAccountPage } from "./pages/CustomerAccountPage";
import { CustomersAdminPage } from "./pages/CustomersAdminPage";
import { DepartmentManagersPage } from "./pages/DepartmentManagersPage";
import { PosCashierPage } from "./pages/PosCashierPage";
import { StorefrontSettingsPage } from "./pages/StorefrontSettingsPage";
import { AdjustmentsPage } from "./pages/AdjustmentsPage";
import { HRPage } from "./pages/HRPage";
import { SecurityLogisticsPage } from "./pages/SecurityLogisticsPage";
import { WebmasterReportsPage } from "./pages/WebmasterReportsPage";
import "./styles.css";

const queryClient = new QueryClient();

function Bootstrap() {
  const hydrate = useAuthStore((s) => s.hydrate);
  const locale = useLocaleStore((s) => s.locale);
  const t = useLocaleStore((s) => s.t);
  const [ready, setReady] = useState(false);

  useEffect(() => {
    document.documentElement.lang = locale;
  }, [locale]);

  useEffect(() => {
    void hydrate().finally(() => setReady(true));
  }, [hydrate]);

  if (!ready) {
    return (
      <main className="login-page">
        <p className="muted">{t("loadingSession")}</p>
      </main>
    );
  }

  return (
    <Routes>
      <Route path="/setup" element={<SetupPage />} />
      <Route path="/upload/:token" element={<MobileUploadPage />} />
      <Route path="/tienda/:slug" element={<PublicStorefrontPage />} />
      <Route path="/tienda/:slug/cuenta" element={<CustomerAccountPage />} />
      <Route path="/login" element={<LoginPage />} />
      <Route
        element={
          <SetupRedirect>
            <RequireAuth />
          </SetupRedirect>
        }
      >
        <Route element={<AppShell />}>
          <Route index element={<DashboardPage />} />
          <Route path="search" element={<SearchPage />} />
          <Route path="inventory" element={<InventoryPage />} />
          <Route path="inventory/receiving" element={<ReceivingPage />} />
          <Route path="inventory/adjustments" element={<AdjustmentsPage />} />
          <Route path="inventory/slips" element={<ShippingSlipsPage />} />
          <Route path="inventory/transport" element={<TransportSheetsPage />} />
          <Route path="inventory/transfers" element={<TransfersPage />} />
          <Route path="inventory/parcels" element={<ParcelsPage />} />
          <Route path="settings/tienda" element={<StorefrontSettingsPage />} />
          <Route path="settings/jefes" element={<DepartmentManagersPage />} />
          <Route path="caja" element={<PosCashierPage />} />
          <Route path="customers" element={<CustomersAdminPage />} />
          <Route path="approvals" element={<ApprovalsPage />} />
          <Route path="mail" element={<MailPage />} />
          <Route path="hr" element={<HRPage />} />
          <Route path="payroll" element={<PayrollPage />} />
          <Route path="reports" element={<ReportsPage />} />
          <Route path="reports/webmaster" element={<WebmasterReportsPage />} />
          <Route path="reports/seguridad" element={<SecurityLogisticsPage />} />
          <Route path="reports/images" element={<ImageReportsPage />} />
        </Route>
      </Route>
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Bootstrap />
      </BrowserRouter>
    </QueryClientProvider>
  );
}
