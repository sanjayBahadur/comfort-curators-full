import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { toast } from "sonner";
import Select from "../components/ui/Select";
import DocumentViewer from "../components/documents/DocumentViewer";
import { getOpsProperties } from "../lib/api/ops";
import { createDocument, getPropertyDocuments } from "../lib/api/owner";
import { humanize, propertyName } from "./ops-format";
import { OwnerGate, OwnerRecordsHeader, OwnerRecordsSkeleton } from "./owner-records";
import "./owner-records.css";

const UPLOAD_DATE = new Intl.DateTimeFormat("en-IN", {
  day: "2-digit",
  month: "short",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
  hour12: false,
  timeZone: "Asia/Kolkata",
});

function uploadedLabel(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : UPLOAD_DATE.format(date);
}

export default function Documents() {
  const [searchParams, setSearchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const propertyQuery = useQuery({ queryKey: ["owner", "properties"], queryFn: getOpsProperties });
  const propertyId = searchParams.get("property") ?? "";
  const documentsQuery = useQuery({
    queryKey: ["owner", "documents", propertyId],
    queryFn: () => getPropertyDocuments(propertyId),
    enabled: Boolean(propertyId),
  });
  const properties = propertyQuery.data?.items ?? [];
  const documents = documentsQuery.data?.items ?? [];

  const [title, setTitle] = useState("");
  const [documentType, setDocumentType] = useState("");

  const createMutation = useMutation({
    mutationFn: createDocument,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["owner", "documents", propertyId] });
      await queryClient.invalidateQueries({ queryKey: ["owner", "dashboard"] });
      setTitle("");
      setDocumentType("");
      toast.success("Document recorded (metadata only)");
    },
  });

  function setProperty(value: string) {
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      if (value) next.set("property", value);
      else next.delete("property");
      return next;
    }, { replace: true });
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!propertyId) return toast.error("Choose a property");
    if (!title.trim()) return toast.error("Title is required");
    if (!documentType.trim()) return toast.error("Document type is required");
    createMutation.mutate({
      property_id: propertyId,
      title: title.trim(),
      document_type: documentType.trim(),
    });
  }

  return (
    <OwnerGate>
      <main className="owner-records registration-frame">
        <OwnerRecordsHeader section="05 / DOCUMENTS" />
        <section className="owner-records-title">
          <div>
            <p>NAME · TYPE · STATUS · UPLOADED</p>
            <h1>Documents</h1>
          </div>
          {propertyId && <strong>PROPERTY SCOPED</strong>}
        </section>

        <section className="owner-records-selector" aria-label="Documents property selector">
          <label>
            <span>PROPERTY</span>
            <Select
              value={propertyId}
              onChange={setProperty}
              options={[
                { value: "", label: "SELECT PROPERTY" },
                ...properties.map((property) => ({ value: property.id, label: propertyName(property) })),
              ]}
            />
          </label>
        </section>

        {!propertyId ? (
          <section className="owner-records-empty"><p>Choose a property above to view and record its documents.</p></section>
        ) : propertyQuery.isLoading ? <OwnerRecordsSkeleton /> : propertyQuery.isError ? (
          <section className="owner-records-error" role="alert"><p>PROPERTIES UNAVAILABLE</p><h2>We could not read your properties.</h2><button type="button" onClick={() => void propertyQuery.refetch()}>TRY AGAIN →</button></section>
        ) : (
          <>
            <form className="owner-records-form" onSubmit={submit} aria-label="Record a document">
              <div className="owner-records-form-fields">
                <label className="owner-records-field">
                  <span>TITLE</span>
                  <input type="text" value={title} onChange={(event) => setTitle(event.currentTarget.value)} placeholder="e.g. Ownership proof" required />
                </label>
                <label className="owner-records-field">
                  <span>DOCUMENT TYPE</span>
                  <input type="text" value={documentType} onChange={(event) => setDocumentType(event.currentTarget.value)} placeholder="e.g. ownership_proof, lease_agreement" required />
                </label>
              </div>
              <div className="owner-records-form-side">
                <p><strong>METADATA ONLY</strong>Upload records the document details — no file is stored in this build.</p>
                <button type="submit" disabled={createMutation.isPending}>
                  {createMutation.isPending ? "RECORDING…" : "RECORD DOCUMENT →"}
                </button>
              </div>
            </form>

            <section className="owner-records-section" aria-labelledby="documents-list-title">
              <header><span>01</span><h2 id="documents-list-title">On file</h2><small>{documentsQuery.isLoading ? "READING" : `${documents.length} DOCUMENT${documents.length === 1 ? "" : "S"}`}</small></header>
              {documentsQuery.isLoading ? <OwnerRecordsSkeleton /> : documentsQuery.isError ? (
                <section className="owner-records-error" role="alert"><p>DOCUMENTS UNAVAILABLE</p><h2>We could not read the documents.</h2><button type="button" onClick={() => void documentsQuery.refetch()}>TRY AGAIN →</button></section>
              ) : documents.length === 0 ? (
                <section className="owner-records-empty"><strong>No documents on file.</strong><p>Record the first document above — only its metadata is stored.</p></section>
              ) : (
                <div className="owner-records-table-wrap">
                  <table className="owner-records-table">
                    <thead><tr><th>NAME</th><th>TYPE</th><th>STATUS</th><th>UPLOADED</th></tr></thead>
                    <tbody>
                      {documents.map((document) => (
                        <tr key={document.id}>
                          <td data-label="NAME"><DocumentViewer document={document.data} formatDate={uploadedLabel} /></td>
                          <td data-label="TYPE">{humanize(document.data.document_type)}</td>
                          <td data-label="STATUS"><span className="owner-records-status" data-status={document.data.status}><i aria-hidden="true" />{humanize(document.data.status)}</span></td>
                          <td data-label="UPLOADED">{uploadedLabel(document.data.created_at)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </section>
          </>
        )}
      </main>
    </OwnerGate>
  );
}
