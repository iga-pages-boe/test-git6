import sys
import re

def read(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def extract_ids(path):
    ids = []
    with open(path, "r", encoding="utf-8") as f:
        depth = 0
        cur_id = ""
        cur_status = ""
        for line in f:
            depth += line.count("{")
            depth -= line.count("}")
            m_id = re.search(r'"ID"\s*:\s*"([^"]+)"', line)
            if m_id:
                cur_id = m_id.group(1)
            m_status = re.search(r'"Status"\s*:\s*"([^"]+)"', line)
            if m_status:
                cur_status = m_status.group(1)
            if depth == 0:
                if cur_id and cur_status == "DeployFailed":
                    ids.append(cur_id)
                cur_id = ""
                cur_status = ""
    return ids

    

def main():
    path = sys.argv[1] if len(sys.argv) > 1 else "iga_with_condition_routes.json"
    for i in extract_ids(path):
        print(i)

if __name__ == "__main__":
    main()
