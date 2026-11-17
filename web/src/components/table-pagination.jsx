import { ChevronLeft, ChevronRight } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Pagination,
  PaginationContent,
  PaginationItem,
} from "@/components/ui/pagination";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100];

export function TablePagination({
  table,
  pageSizeOptions = DEFAULT_PAGE_SIZE_OPTIONS,
}) {
  const { pageIndex, pageSize } = table.getState().pagination;
  const total = table.getFilteredRowModel().rows.length;
  if (total === 0) return null;
  const from = pageIndex * pageSize + 1;
  const to = Math.min((pageIndex + 1) * pageSize, total);
  return (
    <div data-slot="table-pagination" className="mt-3 flex w-full flex-wrap items-center justify-between gap-2 text-control text-faint">
      <div className="flex items-center gap-2">
        <Label className="whitespace-nowrap text-control font-normal text-faint">
          Rows per page:
        </Label>
        <NativeSelect
          value={String(pageSize)}
          onChange={(e) => {
            table.setPageSize(Number(e.target.value));
            table.setPageIndex(0);
          }}
          size="sm"
          className="w-16"
        >
          {pageSizeOptions.map((opt) => (
            <NativeSelectOption key={opt} value={String(opt)}>
              {opt}
            </NativeSelectOption>
          ))}
        </NativeSelect>
      </div>
      <div className="flex items-center gap-2">
        <span className="whitespace-nowrap tabular-nums">
          {from}–{to} of {total}
        </span>
        <Pagination>
          <PaginationContent>
            <PaginationItem>
              <Button
                aria-label="Go to previous page"
                size="icon-sm"
                variant="ghost"
                onClick={() => table.previousPage()}
                disabled={!table.getCanPreviousPage()}
              >
                <ChevronLeft className="size-4" />
              </Button>
            </PaginationItem>
            <PaginationItem>
              <Button
                aria-label="Go to next page"
                size="icon-sm"
                variant="ghost"
                onClick={() => table.nextPage()}
                disabled={!table.getCanNextPage()}
              >
                <ChevronRight className="size-4" />
              </Button>
            </PaginationItem>
          </PaginationContent>
        </Pagination>
      </div>
    </div>
  );
}
