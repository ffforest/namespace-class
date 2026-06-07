#!/usr/bin/env ruby
# frozen_string_literal: true

require "find"
require "yaml"

paths = ARGV
abort "usage: scripts/check-manifests.rb <file-or-dir>..." if paths.empty?

files = paths.flat_map do |path|
  if File.directory?(path)
    found = []
    Find.find(path) do |entry|
      next unless File.file?(entry)
      next unless entry.end_with?(".yaml", ".yml")

      found << entry
    end
    found
  else
    [path]
  end
end.sort

abort "no YAML files found" if files.empty?

errors = []
checked = 0

files.each do |file|
  docs = YAML.load_stream(File.read(file))
  docs.each_with_index do |doc, index|
    next if doc.nil?

    checked += 1
    prefix = "#{file} document #{index + 1}"
    unless doc.is_a?(Hash)
      errors << "#{prefix}: document must be a map"
      next
    end

    %w[apiVersion kind metadata].each do |field|
      errors << "#{prefix}: missing #{field}" unless doc.key?(field)
    end

    metadata = doc["metadata"]
    if !metadata.is_a?(Hash) || metadata["name"].to_s.empty?
      errors << "#{prefix}: missing metadata.name"
    end

    next unless doc["kind"] == "CustomResourceDefinition"

    spec = doc["spec"]
    unless spec.is_a?(Hash)
      errors << "#{prefix}: CRD missing spec"
      next
    end

    %w[group scope names versions].each do |field|
      errors << "#{prefix}: CRD missing spec.#{field}" unless spec.key?(field)
    end
  end
rescue Psych::SyntaxError => e
  errors << "#{file}: YAML syntax error: #{e.message}"
end

if errors.any?
  warn errors.join("\n")
  exit 1
end

puts "checked #{checked} manifest documents"
