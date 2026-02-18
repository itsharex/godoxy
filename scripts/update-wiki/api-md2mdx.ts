export function md2mdx(md: string) {
	const indexFirstH2 = md.indexOf("## ");
	if (indexFirstH2 === -1) {
		console.error("## section not found in the file");
		process.exit(1);
	}

	const h1 = md.slice(0, indexFirstH2);
	const h1Lines = h1.split("\n");
	const keptH1Lines: string[] = [];
	const callouts: string[] = [];

	for (let i = 0; i < h1Lines.length; i++) {
		const line = h1Lines[i] ?? "";
		const calloutStart = line.match(/^>\s*\[!([a-z0-9_-]+)\]\s*$/i);
		if (calloutStart) {
			const rawCalloutType = (calloutStart[1] ?? "note").toLowerCase();
			const calloutType =
				rawCalloutType === "note"
					? "info"
					: rawCalloutType === "warning"
						? "warn"
						: rawCalloutType;
			const contentLines: string[] = [];

			i++;
			for (; i < h1Lines.length; i++) {
				const blockLine = h1Lines[i] ?? "";
				if (!blockLine.startsWith(">")) {
					i--;
					break;
				}
				contentLines.push(blockLine.replace(/^>\s?/, ""));
			}

			while (contentLines[0] === "") {
				contentLines.shift();
			}
			while (contentLines[contentLines.length - 1] === "") {
				contentLines.pop();
			}

			if (contentLines.length > 0) {
				callouts.push(
					`<Callout type="${calloutType}">\n${contentLines.join("\n")}\n</Callout>`,
				);
			}
			continue;
		}

		keptH1Lines.push(line);
	}

	const h1WithoutCallout = keptH1Lines.join("\n");
	const titleMatchResult = h1WithoutCallout.match(
		new RegExp(/^\s*#\s+([^\n]+)/, "im"),
	);
	const title = titleMatchResult?.[1]?.trim() ?? "";
	let description = h1WithoutCallout
		.replace(new RegExp(/^\s*#\s+[^\n]+\n?/, "im"), "")
		.replaceAll(new RegExp(/^\s*>.+$/, "gm"), "")
		.trim();
	// remove trailing full stop
	if (description.endsWith(".")) {
		description = description.slice(0, -1);
	}

	let header = `---\ntitle: ${title}`;
	if (description) {
		header += `\ndescription: ${description}`;
	}
	header += "\n---";

	md = md.slice(indexFirstH2);
	const calloutsBlock = callouts.join("\n\n");
	if (calloutsBlock) {
		md = `${header}\n\n${calloutsBlock}\n\n${md}`;
	} else {
		md = `${header}\n\n${md}`;
	}

	md = md.replaceAll("</br>", "<br/>");
	md = md.replaceAll("<0", "\\<0");

	return md;
}

async function main() {
	const Parser = await import("argparse").then((m) => m.ArgumentParser);

	const parser = new Parser({
		description: "Convert API markdown to VitePress MDX",
	});
	parser.add_argument("-i", "--input", {
		help: "Input markdown file",
		required: true,
	});
	parser.add_argument("-o", "--output", {
		help: "Output VitePress MDX file",
		required: true,
	});

	const args = parser.parse_args();
	const inMdFile = args.input;
	const outMdxFile = args.output;

	const md = await Bun.file(inMdFile).text();
	const mdx = md2mdx(md);
	await Bun.write(outMdxFile, mdx);
}

if (import.meta.main) {
	await main();
}
