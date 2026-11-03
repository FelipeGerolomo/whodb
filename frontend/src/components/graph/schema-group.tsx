/*
 * Copyright 2026 Clidey, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import type { FC } from "react";
import type { NodeProps } from "reactflow";
import { cn } from "@clidey/ux";
import { ph } from "../../utils/privacy";

/**
 * The box an all-schemas graph draws around one schema's tables. Its border and name are sized to
 * stay readable at the low zoom a whole database needs.
 */
export const SchemaGroupNode: FC<NodeProps<{ name: string }>> = ({ data }) => (
    <div className="w-full h-full rounded-[64px] border-[16px] border-dashed" style={{
        borderColor: "color-mix(in srgb, var(--primary) 50%, transparent)",
        background: "color-mix(in srgb, var(--primary) 6%, transparent)",
    }}>
        <h2 className={cn("text-[200px] leading-none font-semibold px-[120px] pt-[80px]", ph.mask)}
            style={{ color: "var(--primary)" }}>{data.name}</h2>
    </div>
);
