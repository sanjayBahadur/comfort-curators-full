import { useState } from "react";
import Modal from "../ui/Modal";
import Popover from "../ui/Popover";
import type { OwnerDocumentData } from "../../lib/api/owner";
import { humanize } from "../../routes/ops-format";
import "./DocumentViewer.css";

type DocumentViewerProps = {
  document: OwnerDocumentData;
  formatDate: (value: string) => string;
};

function StatusLabel({ status }: { status: string }) {
  return (
    <span className="document-viewer-status" data-status={status}>
      <i aria-hidden="true" />
      {humanize(status)}
    </span>
  );
}

export default function DocumentViewer({ document, formatDate }: DocumentViewerProps) {
  const [modalOpen, setModalOpen] = useState(false);
  const [popoverOpen, setPopoverOpen] = useState(false);
  const typeLabel = humanize(document.document_type);
  const statusLabel = humanize(document.status);

  return (
    <div className="document-viewer-row-actions">
      <button className="document-viewer-name" type="button" onClick={() => setModalOpen(true)}>
        <strong>{document.title}</strong>
        <small>{document.id}</small>
      </button>

      <Popover
        open={popoverOpen}
        onClose={() => setPopoverOpen(false)}
        label={`Quick preview of ${document.title}`}
        trigger={
          <button
            className="document-viewer-preview"
            type="button"
            onClick={() => setPopoverOpen((open) => !open)}
          >
            PREVIEW
          </button>
        }
      >
        <div className="document-viewer-mini">
          <p className="document-viewer-mini-label">QUICK PEEK</p>
          <strong>{document.title}</strong>
          <dl>
            <div><dt>TYPE</dt><dd>{typeLabel}</dd></div>
            <div><dt>STATUS</dt><dd><StatusLabel status={document.status} /></dd></div>
            <div><dt>DATE</dt><dd>{formatDate(document.created_at)}</dd></div>
          </dl>
        </div>
      </Popover>

      <Modal
        open={modalOpen}
        onClose={() => setModalOpen(false)}
        title={document.title}
        label="DOCUMENT RECORD / METADATA"
      >
        <div className="document-viewer-modal">
          <div className="document-viewer-heading">
            <span className="document-viewer-type">{typeLabel}</span>
            <StatusLabel status={document.status} />
          </div>
          <p className="document-viewer-notice">
            METADATA ONLY · No file is stored in this build. This view shows the recorded document details.
          </p>
          <dl className="document-viewer-details">
            <div><dt>DOCUMENT TYPE</dt><dd>{typeLabel}</dd></div>
            <div><dt>VERSION</dt><dd>{document.version} / CURRENT {document.current_version}</dd></div>
            <div><dt>CREATED</dt><dd>{formatDate(document.created_at)}</dd></div>
            <div><dt>UPDATED</dt><dd>{formatDate(document.updated_at)}</dd></div>
            {document.expires_at && <div><dt>EXPIRES</dt><dd>{formatDate(document.expires_at)}</dd></div>}
          </dl>
          <small className="document-viewer-id">{document.id}</small>
          <span className="document-viewer-status-text">STATUS · {statusLabel}</span>
        </div>
      </Modal>
    </div>
  );
}
