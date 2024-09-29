Server 建立 TCB，開啟監聽連線，進入狀態 LISTENING
Client 發出連線要求 SYN，進入狀態 SYN-SENT，等待回應
Server 收到 SYN 要求，回應連線傳送 SYN+ACK，並進入狀態 SYN-RCVD (SYN-RECEIVED)
Client 收到 SYN+ACK 確認完成連線進入狀態 ESTABLISHED，並送出 ACK
Server 收到 ACK 確認連線完成，也進入狀態 ESTABLISHED
雙方開始傳送交換資料

Client 準備關閉連線，發出 FIN，進入狀態 FIN-WAIT-1
Server 收到 FIN，發回收到的 ACK，進入狀態 CLOSE-WAIT，並通知 App 準備斷線
Client 收到 ACK，進入狀態 FIN-WAIT-2，等待 server 發出 FIN
Server 確認 App 處理完斷線請求，發出 FIN，並進入狀態 LAST-ACK
Client 收到 FIN，並回傳確認的 ACK，進入狀態 TIME-WAIT，等待時間過後正式關閉連線
Server 收到 ACK，便直接關閉連線
