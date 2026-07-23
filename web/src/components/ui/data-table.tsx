"use client"

import * as React from "react"
import {
  type ColumnDef,
  type PaginationState,
  type RowSelectionState,
  type Table as TanstackTable,
  flexRender,
  getCoreRowModel,
  getPaginationRowModel,
  useReactTable,
} from "@tanstack/react-table"

import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import { Field, FieldLabel } from "@/components/ui/field"
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

declare module "@tanstack/react-table" {
  // TData/TValue required for declaration merging with TanStack Table.
  // eslint-disable-next-line @typescript-eslint/no-unused-vars -- module augmentation signature
  interface ColumnMeta<TData, TValue> {
    className?: string
    headerClassName?: string
  }
}

const PAGE_SIZE_OPTIONS = [10, 25, 50, 100] as const

export function selectColumn<TData>(): ColumnDef<TData> {
  return {
    id: "select",
    header: ({ table }) => (
      <Checkbox
        checked={table.getIsAllPageRowsSelected()}
        indeterminate={
          table.getIsSomePageRowsSelected() &&
          !table.getIsAllPageRowsSelected()
        }
        onCheckedChange={(value) => table.toggleAllPageRowsSelected(!!value)}
        aria-label="Select all"
      />
    ),
    cell: ({ row }) => (
      <Checkbox
        checked={row.getIsSelected()}
        disabled={!row.getCanSelect()}
        onCheckedChange={(value) => row.toggleSelected(!!value)}
        aria-label="Select row"
      />
    ),
    enableSorting: false,
    enableHiding: false,
    meta: { headerClassName: "w-10", className: "w-10" },
  }
}

interface DataTableProps<TData, TValue> {
  columns: ColumnDef<TData, TValue>[]
  data: TData[]
  getRowId?: (row: TData) => string
  enableRowSelection?: boolean
  pageSizeOptions?: readonly number[]
  initialPageSize?: number
  toolbar?: (ctx: {
    selected: TData[]
    table: TanstackTable<TData>
    clearSelection: () => void
  }) => React.ReactNode
}

export function DataTable<TData, TValue>({
  columns,
  data,
  getRowId,
  enableRowSelection = false,
  pageSizeOptions = PAGE_SIZE_OPTIONS,
  initialPageSize = 10,
  toolbar,
}: DataTableProps<TData, TValue>) {
  const [rowSelection, setRowSelection] = React.useState<RowSelectionState>({})
  const [pagination, setPagination] = React.useState<PaginationState>({
    pageIndex: 0,
    pageSize: initialPageSize,
  })

  // TanStack Table returns unstable function refs; React Compiler skips memoizing this.
  // eslint-disable-next-line react-hooks/incompatible-library -- known TanStack Table limitation
  const table = useReactTable({
    data,
    columns,
    getCoreRowModel: getCoreRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    getRowId,
    enableRowSelection,
    onRowSelectionChange: setRowSelection,
    onPaginationChange: setPagination,
    state: { rowSelection, pagination },
  })

  const pageCount = table.getPageCount()
  const selected = table
    .getFilteredSelectedRowModel()
    .rows.map((row) => row.original)
  const clearSelection = () => setRowSelection({})

  return (
    <div className="flex flex-col gap-4">
      {toolbar && selected.length > 0
        ? toolbar({ selected, table, clearSelection })
        : null}
      <div className="overflow-hidden rounded-xl border border-border/60">
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id}>
                {headerGroup.headers.map((header) => (
                  <TableHead
                    key={header.id}
                    className={header.column.columnDef.meta?.headerClassName}
                  >
                    {header.isPlaceholder
                      ? null
                      : flexRender(
                        header.column.columnDef.header,
                        header.getContext(),
                      )}
                  </TableHead>
                ))}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {table.getRowModel().rows?.length ? (
              table.getRowModel().rows.map((row) => (
                <TableRow
                  key={row.id}
                  data-state={row.getIsSelected() ? "selected" : undefined}
                >
                  {row.getVisibleCells().map((cell) => (
                    <TableCell
                      key={cell.id}
                      className={cell.column.columnDef.meta?.className}
                    >
                      {flexRender(
                        cell.column.columnDef.cell,
                        cell.getContext(),
                      )}
                    </TableCell>
                  ))}
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell
                  colSpan={columns.length}
                  className="h-24 text-center"
                >
                  No results.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <Field orientation="horizontal" className="w-auto items-center gap-2">
          <FieldLabel htmlFor="rows-per-page" className="text-muted-foreground">
            Rows per page
          </FieldLabel>
          <Select
            value={String(pagination.pageSize)}
            onValueChange={(value) => {
              if (value != null) table.setPageSize(Number(value))
            }}
          >
            <SelectTrigger
              id="rows-per-page"
              aria-label="Rows per page"
              size="sm"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {pageSizeOptions.map((size) => (
                  <SelectItem key={size} value={String(size)}>
                    {size}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
        <div className="flex items-center gap-2">
          <span className="text-sm text-muted-foreground tabular-nums">
            Page {pageCount === 0 ? 0 : pagination.pageIndex + 1} of {pageCount}
          </span>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => table.previousPage()}
            disabled={!table.getCanPreviousPage()}
          >
            Previous
          </Button>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => table.nextPage()}
            disabled={!table.getCanNextPage()}
          >
            Next
          </Button>
        </div>
      </div>
    </div>
  )
}
